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
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/grundprinzip/spark-connect-proxy/internal/control"

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

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	rpcLogger := logger.With("service", "gRPC/server", "component", "grpc-proxy")
	httpLogger := logger.With("service", "http/server", "component", "control")

	// Create prom registry
	reg := prometheus.NewRegistry()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	reg.MustRegister(srvMetrics)

	g := &run.Group{}

	proxyService := scproxy.NewSparkConnectProxy()
	if err := proxyService.AddKnownBackend("localhost:15002"); err != nil {
		log.Fatalf("Error adding known backend: %v", err)
	}

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
		m := control.CreateServerMux()
		control.RegisterPromMetricsHandler(httpLogger, m, reg)
		control.RegisterSessionHandlers(httpLogger, m, proxyService.State())
		httpSrv.Handler = m
		return httpSrv.ListenAndServe()
	}, func(err error) {
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
