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
	"log"
	"math/rand"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
	b, e := createSparkConnectBackend(backend)
	if e != nil {
		log.Fatalf("Error creating backend %v", e)
		return e
	}
	p.knownBackends = append(p.knownBackends, b)
	return nil
}

func (p *SparkConnectProxy) GetRandomBackend() proxy.Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.knownBackends) == 0 {
		return nil
	}
	idx := rand.Intn(len(p.knownBackends))
	return p.knownBackends[idx]
}

func CreateRouter(service *SparkConnectProxy) proxy.StreamDirector {
	director := func(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
		backend := service.GetRandomBackend()
		return proxy.One2One, []proxy.Backend{backend}, nil
	}
	return director
}

func (p *SparkConnectProxy) CreateStreamHandler() grpc.StreamHandler {
	return proxy.TransparentHandler(CreateRouter(p))
}

func createSparkConnectBackend(dst string) (proxy.Backend, error) {
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
