package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type SparkConnectProxy struct {
	// Mapping of session ID -> backend URL
	sessionBackends map[string]*url.URL
	// List of known backends from which new sessions pick randomly
	knownBackends []*url.URL

	// Protects sessionBackends and knownBackends
	mu sync.RWMutex

	// Metrics registry and specific counters
	registry  *prometheus.Registry
	requests  prometheus.Counter
	successes prometheus.Counter
	failures  prometheus.Counter
}

// NewSparkConnectProxy sets up our proxy with an empty routing table and a metrics registry.
func NewSparkConnectProxy() *SparkConnectProxy {
	// Create a metrics registry.
	r := prometheus.NewRegistry()

	// Create (or get) our counters.
	rc := promauto.NewCounter(prometheus.CounterOpts{
		Name: "request_counter",
		Help: "The total number of processed requests",
	})

	sc := promauto.NewCounter(prometheus.CounterOpts{
		Name: "success_counter",
		Help: "The total number of processed requests",
	})

	fc := promauto.NewCounter(prometheus.CounterOpts{
		Name: "fail_counter",
		Help: "The total number of processed requests",
	})

	return &SparkConnectProxy{
		sessionBackends: make(map[string]*url.URL),
		knownBackends:   []*url.URL{},
		registry:        r,
		requests:        rc,
		successes:       sc,
		failures:        fc,
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
	p.requests.Inc()

	sessionID := r.Header.Get("x-spark-connect-session-id")
	if sessionID == "" {
		// Missing session ID => fail
		p.failures.Inc()
		http.Error(w, "Missing x-spark-connect-session-id header", http.StatusBadRequest)
		return
	}

	backend := p.getRandomBackend()
	if backend == nil {
		// No backend => fail
		p.failures.Inc()
		http.Error(w, "No backend found for the provided session ID", http.StatusNotFound)
		return
	}

	// We have a valid session/back-end mapping => success (from the proxy's perspective)
	p.successes.Inc()

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

// handleMetrics returns both session-backend info AND the standard Codahale JSON from go-metrics.
func (p *SparkConnectProxy) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// 1) Gather session info into a map
	type sessionData struct {
		TotalSessions int                 `json:"total_sessions"`
		Backends      map[string][]string `json:"backends"`
	}
	sessInfo := sessionData{
		Backends: make(map[string][]string),
	}

	p.mu.RLock()
	for sid, be := range p.sessionBackends {
		sessInfo.TotalSessions++
		beStr := be.String()
		sessInfo.Backends[beStr] = append(sessInfo.Backends[beStr], sid)
	}
	p.mu.RUnlock()

	// 2) Get the "standard" Codahale-style metrics JSON from go-metrics
	//    We'll write into a buffer, then unmarshal it into an `interface{}`
	//    so we can embed it in our final JSON response.
	var buf bytes.Buffer
	w.Header().Set("Content-Type", "application/json")
	// Write standard metrics JSON (counters, gauges, histograms, etc.) into `buf`
	// metricsjson.WriteJSONOnce(p.registry, time.Now(), &buf)

	// Unmarshal that partial JSON into a generic interface
	var codahale interface{}
	if err := json.Unmarshal(buf.Bytes(), &codahale); err != nil {
		// In case of any error, just return a 500
		http.Error(w, fmt.Sprintf("Unable to serialize metrics: %v", err), http.StatusInternalServerError)
		return
	}

	// 3) Combine session info + codahale metrics into a single JSON object
	combined := map[string]interface{}{
		"sessions": sessInfo,
		"metrics":  codahale,
	}

	// 4) Write out the final JSON
	_ = json.NewEncoder(w).Encode(combined)
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

	// Register some known backends
	sparkURL1, _ := url.Parse("http://127.0.0.1:15002")
	proxy.AddKnownBackend(sparkURL1)

	// Create HTTP server that uses h2c for gRPC
	srv := &http.Server{
		Addr:    ":8080",
		Handler: h2c.NewHandler(http.HandlerFunc(proxy.mainHandler), &http2.Server{}),
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
