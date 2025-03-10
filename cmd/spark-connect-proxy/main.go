// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grundprinzip/spark-connect-proxy/connect"

	"github.com/grundprinzip/spark-connect-proxy/internal/config"
	"github.com/grundprinzip/spark-connect-proxy/internal/control"
	"github.com/grundprinzip/spark-connect-proxy/internal/errors"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"

	scproxy "github.com/grundprinzip/spark-connect-proxy/internal/proxy"
)

// interceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func interceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func convertLogLevel(level *string) slog.Level {
	switch *level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupServerCredentials creates gRPC server credentials based on TLS configuration
func setupServerCredentials(tlsConfig config.TLSConfig, logger *slog.Logger) grpc.ServerOption {
	if !tlsConfig.Enabled {
		return grpc.Creds(insecure.NewCredentials())
	}

	// Validate required TLS files
	if tlsConfig.CertFile == "" || tlsConfig.KeyFile == "" {
		logger.Error("Missing certificate or key file when TLS is enabled")
		os.Exit(1)
	}

	// Load certificate and private key
	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		logger.Error("Failed to load TLS certificate or key", "error", err)
		os.Exit(1)
	}

	// Create TLS credentials
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Set server name if provided
	if tlsConfig.ServerName != "" {
		config.ServerName = tlsConfig.ServerName
	}

	return grpc.Creds(credentials.NewTLS(config))
}

// getListenAddress returns the configured listen address or the default if not set
func getListenAddress(configuredAddr string) string {
	if configuredAddr != "" {
		return configuredAddr
	}
	return "localhost:8080"
}

func main() {
	// Parse the command line flags.
	configFile := flag.String("config-file", "spark-connect-proxy.yaml", "The configuration file to use.")
	logLevel := flag.String("log-level", "info", "The log level to use.")
	flag.Parse()

	// Set up the basic loggers.
	mutLogLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: mutLogLevel,
	}))
	rpcLogger := logger.With("service", "gRPC/server", "component", "grpc-proxy")
	httpLogger := logger.With("service", "http/server", "component", "control")

	// Load default configuration file. If the file is not found, exit.
	// The configuration file is responsible as well for instantiating the backend providers.
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Error("Error loading configuration file", "file", *configFile, "error", err)
		os.Exit(1)
	}

	// Update the log level if necessary:
	if cfg.LogLevel != "" {
		mutLogLevel.Set(convertLogLevel(&cfg.LogLevel))
	} else {
		mutLogLevel.Set(convertLogLevel(logLevel))
	}

	// Check for the backends
	provider := cfg.BackendProvider
	proxyService := scproxy.NewSparkConnectProxy(provider.Spec.(connect.BackendProvider), cfg.LoadPolicy, rpcLogger)

	// Create prom registry for metrics.
	reg := prometheus.NewRegistry()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	reg.MustRegister(srvMetrics)

	g := &run.Group{}

	// Initialize server credentials based on TLS configuration
	creds := setupServerCredentials(cfg.Server.TLS, rpcLogger)

	server := grpc.NewServer(
		creds,
		grpc.ForceServerCodecV2(proxy.Codec()),
		grpc.UnknownServiceHandler(proxyService.CreateStreamHandler()),
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
		// Use configuration for listen address or default
		listenAddr := getListenAddress(cfg.Server.ListenAddr)

		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return errors.WithStringf(err, "failed to listen on %s", listenAddr)
		}

		secureMode := "insecure"
		if cfg.Server.TLS.Enabled {
			secureMode = "secure"
		}
		rpcLogger.Info(fmt.Sprintf("Starting %s gRPC server", secureMode), "address", listenAddr)
		return server.Serve(lis)
	}, func(err error) {
		rpcLogger.Info("Stopping GRPC Proxy service...")
		server.GracefulStop()
		server.Stop()
	})

	// Create the HTTP server for Prometheus and control endpoints
	httpSrv := &http.Server{
		Addr: ":8081",
	}

	// Add the HTTP server to the run group.
	g.Add(func() error {
		m := control.CreateServerMux()
		control.RegisterPromMetricsHandler(httpLogger, m, reg)
		control.RegisterSessionHandlers(httpLogger, m, proxyService.State())
		httpSrv.Handler = m

		secureMode := "insecure"
		if cfg.Server.TLS.Enabled {
			secureMode = "secure"
		}
		httpLogger.Info(fmt.Sprintf("Starting %s HTTP control server", secureMode), "address", httpSrv.Addr)

		if cfg.Server.TLS.Enabled {
			return httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		}
		return httpSrv.ListenAndServe()
	}, func(err error) {
		httpLogger.Info("Stopping Control HTTP service...")
		if err := httpSrv.Close(); err != nil {
			log.Fatalf("failed to stop web server: %v", err)
		}
	})

	// Add ctrl-c handler
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		log.Printf("program interrupted: %v", err)
		os.Exit(1)
	}
}
