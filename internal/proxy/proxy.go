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
}

func (p *SparkConnectProxy) State() *ProxyState {
	return p.proxyState
}

// NewSparkConnectProxy sets up our proxy with an empty routing table and a metrics registry.
func NewSparkConnectProxy(bp connect.BackendProvider) *SparkConnectProxy {
	// Create a metrics registry.
	r := prometheus.NewRegistry()

	return &SparkConnectProxy{
		proxyState: NewProxyState(bp),
		registry:   r,
	}
}

func CreateRouter(service *SparkConnectProxy) proxy.StreamDirector {
	director := func(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
		// Extract metadata from the context.
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return proxy.One2One, nil, errors.WithString(errors.ProxyError, "no metadata found in context")
		}
		// Lookup the session from the context.
		sessionIDs := md.Get(HEADER_SPARK_SESSION_ID)
		if len(sessionIDs) != 1 {
			return proxy.One2One, nil, errors.WithString(errors.ProxyError, "no session ID found in metadata")
		}

		// Lookup the backend for the session.
		sessionID := sessionIDs[0]
		backend, err := service.proxyState.GetBackendForSession(sessionID)
		return proxy.One2One, []proxy.Backend{backend}, err
	}
	return director
}

func (p *SparkConnectProxy) CreateStreamHandler() grpc.StreamHandler {
	return proxy.TransparentHandler(CreateRouter(p))
}
