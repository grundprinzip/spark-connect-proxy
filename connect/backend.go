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

package connect

import (
	"context"

	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// LoadPolicy is an interface that defines how backends are loaded and how
// work is going to be distributed among them.
type LoadPolicy interface {
	// Next returns the next backend to use. This is only called when
	// a new session is started.
	Next() (Backend, error)
	// Release is called when a session is closed. The load policy
	// implementation can choose to release the backend or keep it.
	Release(id string) error
}

type Backend interface {
	ID() string
	Url() string
	Connection() (proxy.Backend, error)
}

type BackendProvider interface {
	Start() (Backend, error)
	Stop(id string) error
	List() ([]Backend, error)
	Size() int
	Get(id string) (Backend, error)
}

func CreateSimpleProxyBackend(dst string) (proxy.Backend, error) {
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
