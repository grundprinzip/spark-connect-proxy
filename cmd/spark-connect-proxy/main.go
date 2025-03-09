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
	proxyService := scproxy.NewSparkConnectProxy(provider.Spec.(connect.BackendProvider), cfg.LoadPolicy)

	// Create prom registry for metrics.
	reg := prometheus.NewRegistry()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	reg.MustRegister(srvMetrics)

	g := &run.Group{}

	server := grpc.NewServer(
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
		listenAddr := "localhost:8080"
		if cfg.Server.ListenAddr != "" {
			listenAddr = cfg.Server.ListenAddr
		}

		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return errors.WithStringf(err, "failed to listen on %s", listenAddr)
		}

		// Check if TLS is enabled
		if cfg.Server.TLS.Enabled {
			// Validate required TLS files
			if cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
				return errors.WithStringf(
					fmt.Errorf("missing certificate or key file"),
					"TLS is enabled but cert_file or key_file is missing",
				)
			}

			rpcLogger.Info("Starting secure gRPC server", "address", listenAddr)
			// Load certificate and private key
			cert, err := tls.LoadX509KeyPair(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
			if err != nil {
				return errors.WithStringf(err, "failed to load TLS certificate or key: %v", err)
			}

			// Create TLS credentials
			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}

			// Set server name if provided
			if cfg.Server.TLS.ServerName != "" {
				tlsConfig.ServerName = cfg.Server.TLS.ServerName
			}

			// Use TLS listener directly with the TLS config
			return server.Serve(tls.NewListener(lis, tlsConfig))
		}

		rpcLogger.Info("Starting insecure gRPC server", "address", listenAddr)
		return server.Serve(lis)
	}, func(err error) {
		rpcLogger.Info("Stopping GRPC Proxy service...")
		server.GracefulStop()
		server.Stop()
	})

	// Create the http server for prometheus
	httpSrv := &http.Server{
		Addr: ":8081",
	}

	// Add the http server to the run group.
	g.Add(func() error {
		m := control.CreateServerMux()
		control.RegisterPromMetricsHandler(httpLogger, m, reg)
		control.RegisterSessionHandlers(httpLogger, m, proxyService.State())
		httpSrv.Handler = m

		// If TLS is enabled for gRPC, also use it for the HTTP control server
		if cfg.Server.TLS.Enabled {
			// Use the same certificate and key
			httpLogger.Info("Starting secure HTTP control server", "address", httpSrv.Addr)
			return httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		}

		httpLogger.Info("Starting insecure HTTP control server", "address", httpSrv.Addr)
		return httpSrv.ListenAndServe()
	}, func(err error) {
		httpLogger.Info("Stopping Control HTTP service...")
		if err := httpSrv.Close(); err != nil {
			log.Fatalf("failed to stop web server: %v", err)
		}
	})

	// Add ctr-c handler
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		log.Printf("program interrupted: %v", err)
		os.Exit(1)
	}
}
