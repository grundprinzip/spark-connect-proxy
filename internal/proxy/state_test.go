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
	"fmt"
	"testing"

	"github.com/grundprinzip/spark-connect-proxy/connect"
	"github.com/siderolabs/grpc-proxy/proxy"
	"github.com/stretchr/testify/assert"
)

type testBackend struct {
	id  string
	url string
}

func (b *testBackend) ID() string {
	return b.id
}

func (b *testBackend) Url() string {
	return b.url
}

func (b *testBackend) Connection() (proxy.Backend, error) {
	return nil, nil
}

type testBackendProvider struct {
	name        string
	StartCalled int
	StopCalled  int
	ListCalled  int
	Backends    []connect.Backend
}

func (p *testBackendProvider) Start() (connect.Backend, error) {
	p.StartCalled++
	p.Backends = append(p.Backends, &testBackend{
		id: fmt.Sprintf("test_%v", p.StartCalled), url: "http://localhost:8080",
	})
	return p.Backends[len(p.Backends)-1], nil
}

func (p *testBackendProvider) Stop(id string) error {
	p.StopCalled++
	// Remove the backend with this id
	for i, b := range p.Backends {
		if b.ID() == id {
			p.Backends = append(p.Backends[:i], p.Backends[i+1:]...)
			return nil
		}
	}
	return nil
}

func (p *testBackendProvider) List() ([]connect.Backend, error) {
	return p.Backends, nil
}

func (p *testBackendProvider) Size() int {
	return len(p.Backends)
}

func (p *testBackendProvider) Get(id string) (connect.Backend, error) {
	for _, b := range p.Backends {
		if b.ID() == id {
			return b, nil
		}
	}
	return nil, fmt.Errorf("Backend not found")
}

func TestRoundRobinPolicy_Next(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &RoundRobinPolicy{bp: provider}
	be, err := p.Next()
	assert.Equal(t, provider.StartCalled, 1)
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be.ID())

	be, err = p.Next()
	assert.Equal(t, provider.StartCalled, 1)
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be.ID())
}

func TestOneToOnePolicy_Next(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &OneToOnePolicy{bp: provider}
	be, err := p.Next()
	assert.Equal(t, provider.StartCalled, 1)
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be.ID())

	be, err = p.Next()
	assert.Equal(t, provider.StartCalled, 2)
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be.ID())
}

func TestMaxSessionsPolicy_Next(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &MaxSessionsPolicy{bp: provider, maxSize: 2, sessions: map[string]int{}}
	be, err := p.Next()
	assert.Equal(t, provider.StartCalled, 1)
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be.ID())

	be, err = p.Next()
	assert.Equal(t, provider.StartCalled, 1)
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be.ID())
	assert.Equal(t, provider.Size(), 1)

	// Next should return the first backend again
	be, err = p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be.ID())
	assert.Equal(t, provider.StartCalled, 2)
	assert.Equal(t, provider.Size(), 2)

	// Release one session
	err = p.Release("test_1")
	assert.NoError(t, err)
	assert.Equal(t, provider.StopCalled, 0)
	assert.Equal(t, provider.Size(), 2)

	// Release one more session for the first backend.
	err = p.Release("test_1")
	assert.NoError(t, err)
	assert.Equal(t, provider.StopCalled, 1)
	assert.Equal(t, provider.Size(), 1)

	// Now releasing it again will fail.
	err = p.Release("test_1")
	assert.Error(t, err)
}
