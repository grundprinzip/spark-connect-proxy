package main

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"log"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
)

type SparkConnectProxy struct {
	// List of known backends from which new sessions pick randomly
	knownBackends []proxy.Backend

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
		knownBackends: make([]proxy.Backend, 0),
		registry:      r,
	}
}

func (p *SparkConnectProxy) AddKnownBackend(backend string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, e := CreateSparkConnectBackend(backend)
	if e != nil {
		log.Fatalf("Error creating backend", e)
		return e
	}
	p.knownBackends = append(p.knownBackends, b)
	return nil
}

func (p *SparkConnectProxy) getRandomBackend() proxy.Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.knownBackends) == 0 {
		return nil
	}
	idx := rand.Intn(len(p.knownBackends))
	return p.knownBackends[idx]
}

func CreateSparkConnectBackend(dst string) (proxy.Backend, error) {
	conn, err := grpc.NewClient(dst,
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(proxy.Codec())),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &proxy.SingleBackend{
		GetConn: func(ctx context.Context) (context.Context, *grpc.ClientConn, error) {
			md, _ := metadata.FromIncomingContext(ctx)

			// Copy the inbound metadata explicitly.
			outCtx := metadata.NewOutgoingContext(ctx, md.Copy())

			return outCtx, conn, nil
		},
	}, nil
}

func CreateRouter(service *SparkConnectProxy) proxy.StreamDirector {
	director := func(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
		backend := service.getRandomBackend()
		return proxy.One2One, []proxy.Backend{backend}, nil
	}
	return director
}

// interceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func interceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func main() {
	rand.Seed(time.Now().UnixNano())

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	rpcLogger := logger.With("service", "gRPC/server", "component", "grpc-proxy")

	// Create prom registry
	reg := prometheus.NewRegistry()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	reg.MustRegister(srvMetrics)

	g := &run.Group{}

	proxyService := NewSparkConnectProxy()
	proxyService.AddKnownBackend("localhost:15002")
	router := CreateRouter(proxyService)

	server := grpc.NewServer(
		grpc.ForceServerCodecV2(proxy.Codec()),
		grpc.UnknownServiceHandler(
			proxy.TransparentHandler(router),
		),
		grpc.ChainUnaryInterceptor(
			logging.UnaryServerInterceptor(interceptorLogger(rpcLogger)),
			srvMetrics.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			logging.StreamServerInterceptor(interceptorLogger(rpcLogger)),
			srvMetrics.StreamServerInterceptor()),
	)

	// Add the GRPC Server to the run group.
	g.Add(func() error {
		lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 8080))
		if err != nil {
			return err
		}
		return server.Serve(lis)
	}, func(err error) {
		server.GracefulStop()
		server.Stop()
	})

	// Create the http server for prometheus
	httpSrv := &http.Server{
		Addr: ":8081",
	}

	// Add the http server to the run group.
	g.Add(func() error {
		m := http.NewServeMux()
		m.Handle("/control/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		httpSrv.Handler = m
		return httpSrv.ListenAndServe()
	}, func(err error) {
		if err := httpSrv.Close(); err != nil {
			log.Fatalf("failed to stop web server", "err", err)
		}
	})

	// Add ctr-c handler
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		log.Printf("program interrupted", "err", err)
		os.Exit(1)
	}
}
