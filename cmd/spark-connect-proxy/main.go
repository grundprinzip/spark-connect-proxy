package main

import (
	"crypto/tls"
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/slok/go-http-metrics/middleware/std"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"net/http/httputil"

	"github.com/prometheus/client_golang/prometheus"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
)

type SparkConnectProxy struct {
	// Mapping of session ID -> backend URL
	sessionBackends map[string]*url.URL
	// List of known backends from which new sessions pick randomly
	knownBackends []*url.URL

	// Protects sessionBackends and knownBackends
	mu sync.RWMutex

	// Metrics registry and specific counters
	registry *prometheus.Registry
}

// NewSparkConnectProxy sets up our proxy with an empty routing table and a metrics registry.
func NewSparkConnectProxy() *SparkConnectProxy {
	// Create a metrics registry.
	r := prometheus.NewRegistry()

	return &SparkConnectProxy{
		sessionBackends: make(map[string]*url.URL),
		knownBackends:   []*url.URL{},
		registry:        r,
	}
}

func (p *SparkConnectProxy) AddKnownBackend(backend *url.URL) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.knownBackends = append(p.knownBackends, backend)
}

func (p *SparkConnectProxy) AddOrUpdateSessionBackend(sessionID string, backendURL *url.URL) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionBackends[sessionID] = backendURL
}

func (p *SparkConnectProxy) RemoveSessionBackend(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessionBackends, sessionID)
}

func (p *SparkConnectProxy) getBackendForSession(sessionID string) *url.URL {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionBackends[sessionID]
}

func (p *SparkConnectProxy) getRandomBackend() *url.URL {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.knownBackends) == 0 {
		return nil
	}
	idx := rand.Intn(len(p.knownBackends))
	return p.knownBackends[idx]
}

// serveGRPC proxies incoming gRPC calls to the correct backend, tracking metrics for requests/failures/successes.
func (p *SparkConnectProxy) serveGRPC(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("x-spark-connect-session-id")
	if sessionID == "" {
		// Missing session ID => fail
		http.Error(w, "Missing x-spark-connect-session-id header", http.StatusBadRequest)
		return
	}

	backend := p.getRandomBackend()
	if backend == nil {
		// No backend => fail
		http.Error(w, "No backend found for the provided session ID", http.StatusNotFound)
		return
	}

	// Forward the request to the backend
	proxy := httputil.NewSingleHostReverseProxy(backend)
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		// Use plain TCP for insecure connections.
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	} // For gRPC over HTTP/2
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = backend.Host
		req.URL.Host = backend.Host
		req.URL.Scheme = backend.Scheme
	}
	proxy.ServeHTTP(w, r)
}

// handleNewSession picks a random backend, creates a new session ID, and associates it.
func (p *SparkConnectProxy) handleNewSession(w http.ResponseWriter, r *http.Request) {
	backend := p.getRandomBackend()
	if backend == nil {
		http.Error(w, "No backends available", http.StatusServiceUnavailable)
		return
	}

	sessionID := uuid.NewString()
	p.AddOrUpdateSessionBackend(sessionID, backend)

	resp := map[string]string{"session_id": sessionID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// controlHandler dispatches to endpoints under /control
func (p *SparkConnectProxy) controlHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/control/session/new":
		p.handleNewSession(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/control/metrics":
		// Export the prom handler
		promhttp.Handler().ServeHTTP(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// mainHandler routes /control vs. everything else (gRPC).
func (p *SparkConnectProxy) mainHandler(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/control" {
		p.controlHandler(w, r)
	} else {
		p.serveGRPC(w, r)
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	proxy := NewSparkConnectProxy()

	// Create our middleware.
	mdlw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{
			// Pass the pre-configured registry.
			Registry: proxy.registry,
		}),
	})

	// Register some known backends
	sparkURL1, _ := url.Parse("http://127.0.0.1:15002")
	proxy.AddKnownBackend(sparkURL1)

	// Create HTTP server that uses h2c for gRPC
	proxyFunc := http.HandlerFunc(proxy.mainHandler)
	// wrap the proxy function using the HTTP middleware that is needed to expose metrics.
	wrappedHandler := std.Handler("", mdlw, proxyFunc)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: h2c.NewHandler(wrappedHandler, &http2.Server{}),
	}

	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", srv.Addr, err)
	}

	log.Printf("Spark Connect gRPC proxy listening on %s", srv.Addr)
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
