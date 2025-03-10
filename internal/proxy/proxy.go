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

package proxy

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/grundprinzip/spark-connect-proxy/internal/config"

	"github.com/grundprinzip/spark-connect-proxy/connect"
	"github.com/grundprinzip/spark-connect-proxy/internal/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type SparkConnectProxy struct {
	proxyState *ProxyState

	// Metrics registry and specific counters
	registry *prometheus.Registry

	// Logger for proxy operations
	logger *slog.Logger
}

func (p *SparkConnectProxy) State() *ProxyState {
	return p.proxyState
}

// NewSparkConnectProxy sets up our proxy with an empty routing table and a metrics registry.
func NewSparkConnectProxy(bp connect.BackendProvider, lp config.LoadPolicyConfig, logger *slog.Logger) *SparkConnectProxy {
	// Create a metrics registry.
	r := prometheus.NewRegistry()

	// Create the proxy with appropriate logger
	proxy := &SparkConnectProxy{
		registry: r,
		logger:   logger,
	}

	// Load policy based on configuration
	var loadPolicy connect.LoadPolicy
	switch lp.Type {
	case "ROUND_ROBIN":
		policy := &RoundRobinPolicy{bp: bp}
		if logger != nil {
			policy.SetLogger(logger.With("component", "load_policy", "type", "round_robin"))
		}
		loadPolicy = policy
	case "ONE_TO_ONE":
		policy := &OneToOnePolicy{bp: bp}
		if logger != nil {
			policy.SetLogger(logger.With("component", "load_policy", "type", "one_to_one"))
		}
		loadPolicy = policy
	case "MAX_SESSIONS":
		maxSize := 5 // Default
		if val, ok := lp.Params["max_size"]; ok {
			if maxSizeParsed, err := strconv.Atoi(val); err == nil {
				maxSize = maxSizeParsed
			}
		}
		policy := &MaxSessionsPolicy{
			bp:       bp,
			maxSize:  maxSize,
			sessions: make(map[string]int),
		}
		if logger != nil {
			policy.SetLogger(logger.With(
				"component", "load_policy",
				"type", "max_sessions",
				"max_size", maxSize,
			))
		}
		loadPolicy = policy
	default:
		// Default to round robin
		policy := &RoundRobinPolicy{bp: bp}
		if logger != nil {
			policy.SetLogger(logger.With("component", "load_policy", "type", "round_robin"))
		}
		loadPolicy = policy
	}

	// Set the logger on the backend provider
	if logger != nil && bp != nil {
		bp.SetLogger(logger.With("component", "backend_provider"))
	}

	// Create the proxy state
	proxy.proxyState = NewProxyState(bp, loadPolicy, logger.With("component", "proxy_state"))

	if logger != nil {
		logger.Info("Spark Connect Proxy initialized", "load_policy", lp.Type)
	}

	return proxy
}

func CreateRouter(service *SparkConnectProxy) proxy.StreamDirector {
	director := func(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
		// Extract metadata from the context.
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			if service.logger != nil {
				service.logger.Warn("No metadata found in context", "method", fullMethodName)
			}
			return proxy.One2One, nil, errors.WithString(errors.ProxyError, "no metadata found in context")
		}

		// Lookup the session from the context.
		sessionIDs := md.Get(HEADER_SPARK_SESSION_ID)
		if len(sessionIDs) != 1 {
			if service.logger != nil {
				service.logger.Warn("No session ID found in metadata",
					"method", fullMethodName,
					"session_ids_count", len(sessionIDs))
			}
			return proxy.One2One, nil, errors.WithString(errors.ProxyError, "no session ID found in metadata")
		}

		// Lookup the backend for the session.
		sessionID := sessionIDs[0]
		backend, err := service.proxyState.GetBackendForSession(sessionID)

		if service.logger != nil {
			if err != nil {
				service.logger.Error("Failed to get backend for session",
					"method", fullMethodName,
					"session_id", sessionID,
					"error", err)
			} else {
				service.logger.Debug("Routing request",
					"method", fullMethodName,
					"session_id", sessionID)
			}
		}

		return proxy.One2One, []proxy.Backend{backend}, err
	}
	return director
}

func (p *SparkConnectProxy) CreateStreamHandler() grpc.StreamHandler {
	return proxy.TransparentHandler(CreateRouter(p))
}
