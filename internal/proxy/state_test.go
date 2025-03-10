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
	"log/slog"
	"testing"

	"github.com/grundprinzip/spark-connect-proxy/connect"
	"github.com/siderolabs/grpc-proxy/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// Return a mock proxy backend
	return &proxy.SingleBackend{}, nil
}

type testBackendProvider struct {
	name        string
	StartCalled int
	StopCalled  int
	ListCalled  int
	GetCalled   int
	Backends    []connect.Backend
	logger      *slog.Logger
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
	p.ListCalled++
	return p.Backends, nil
}

func (p *testBackendProvider) Size() int {
	return len(p.Backends)
}

func (p *testBackendProvider) Get(id string) (connect.Backend, error) {
	p.GetCalled++
	for _, b := range p.Backends {
		if b.ID() == id {
			return b, nil
		}
	}
	return nil, fmt.Errorf("Backend not found")
}

func (p *testBackendProvider) SetLogger(logger *slog.Logger) {
	p.logger = logger
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

func TestRoundRobinPolicy_MultipleBackends(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &RoundRobinPolicy{bp: provider}

	// Start with an empty set
	assert.Equal(t, provider.Size(), 0)

	// First call will start a backend
	be1, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, provider.StartCalled, 1)
	assert.Equal(t, "test_1", be1.ID())

	// Add another backend manually
	be2, err := provider.Start()
	assert.NoError(t, err)
	assert.Equal(t, provider.StartCalled, 2)
	assert.Equal(t, "test_2", be2.ID())

	// Should cycle to the second backend
	be3, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be3.ID())

	// Should cycle back to the first backend
	be4, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be4.ID())
}

func TestRoundRobinPolicy_Release(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &RoundRobinPolicy{bp: provider}

	// Start a backend
	be, err := p.Next()
	assert.NoError(t, err)

	// Release should do nothing for RoundRobin
	err = p.Release(be.ID())
	assert.NoError(t, err)
	assert.Equal(t, provider.StopCalled, 0)
	assert.Equal(t, provider.Size(), 1)
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

func TestOneToOnePolicy_Release(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &OneToOnePolicy{bp: provider}

	// Start a backend
	be, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, provider.StartCalled, 1)
	assert.Equal(t, provider.Size(), 1)

	// Release should stop the backend
	err = p.Release(be.ID())
	assert.NoError(t, err)
	assert.Equal(t, provider.StopCalled, 1)
	assert.Equal(t, provider.Size(), 0)

	// Create a new backend
	newBackend, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, provider.StartCalled, 2)
	assert.Equal(t, provider.Size(), 1)
	assert.NotEmpty(t, newBackend.ID())
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

func TestMaxSessionsPolicy_ThresholdRespect(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &MaxSessionsPolicy{bp: provider, maxSize: 3, sessions: map[string]int{}}

	// First backend
	be1, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be1.ID())
	assert.Equal(t, p.sessions["test_1"], 1)

	// Add two more sessions to first backend
	be2, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be2.ID())
	assert.Equal(t, p.sessions["test_1"], 2)

	be3, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be3.ID())
	assert.Equal(t, p.sessions["test_1"], 3)

	// At capacity, next call should create a new backend
	be4, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be4.ID())
	assert.Equal(t, p.sessions["test_2"], 1)
	assert.Equal(t, provider.Size(), 2)
}

func TestMaxSessionsPolicy_MultiBackendBalance(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	p := &MaxSessionsPolicy{bp: provider, maxSize: 2, sessions: map[string]int{}}

	// Create two backends with one session each
	be1, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_1", be1.ID())

	// Manually create another backend
	be2, err := provider.Start()
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be2.ID())
	p.sessions["test_2"] = 1

	// Third session should go to backend with fewest sessions
	be3, err := p.Next()
	assert.NoError(t, err)
	// Both have 1 session, so it should pick the first backend
	assert.Equal(t, "test_1", be3.ID())
	assert.Equal(t, p.sessions["test_1"], 2)
	assert.Equal(t, p.sessions["test_2"], 1)

	// Fourth session should go to backend with fewest sessions
	be4, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_2", be4.ID())
	assert.Equal(t, p.sessions["test_1"], 2)
	assert.Equal(t, p.sessions["test_2"], 2)

	// Fifth session should need a new backend
	be5, err := p.Next()
	assert.NoError(t, err)
	assert.Equal(t, "test_3", be5.ID())
	assert.Equal(t, p.sessions["test_3"], 1)
}

func TestGetBackendForSession(t *testing.T) {
	provider := &testBackendProvider{name: "test"}
	loadPolicy := &OneToOnePolicy{bp: provider}
	state := NewProxyState(provider, loadPolicy, nil)

	// Start a session
	sessionID, err := state.StartSession()
	require.NoError(t, err)

	// Get backend for this session
	backend, err := state.GetBackendForSession(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, 1, provider.GetCalled)

	// Try getting backend for non-existent session
	_, err = state.GetBackendForSession("non-existent-session")
	assert.Error(t, err)
}

func TestProxyState_WithDifferentLoadPolicies(t *testing.T) {
	testCases := []struct {
		name             string
		loadPolicy       connect.LoadPolicy
		sessions         int
		expectedBackends int
	}{
		{
			name:             "OneToOnePolicy",
			loadPolicy:       &OneToOnePolicy{bp: &testBackendProvider{name: "test"}},
			sessions:         3,
			expectedBackends: 3,
		},
		{
			name:             "RoundRobinPolicy",
			loadPolicy:       &RoundRobinPolicy{bp: &testBackendProvider{name: "test"}},
			sessions:         3,
			expectedBackends: 1,
		},
		{
			name: "MaxSessionsPolicy",
			loadPolicy: &MaxSessionsPolicy{
				bp:       &testBackendProvider{name: "test"},
				maxSize:  2,
				sessions: map[string]int{},
			},
			sessions:         3,
			expectedBackends: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Cast to get access to the backend provider
			var bp *testBackendProvider

			switch lp := tc.loadPolicy.(type) {
			case *OneToOnePolicy:
				bp = lp.bp.(*testBackendProvider)
			case *RoundRobinPolicy:
				bp = lp.bp.(*testBackendProvider)
			case *MaxSessionsPolicy:
				bp = lp.bp.(*testBackendProvider)
			default:
				t.Fatalf("Unknown load policy type: %T", tc.loadPolicy)
			}

			state := NewProxyState(bp, tc.loadPolicy, nil)

			// Start multiple sessions
			sessionIDs := make([]string, 0, tc.sessions)
			for i := 0; i < tc.sessions; i++ {
				sessionID, err := state.StartSession()
				require.NoError(t, err)
				sessionIDs = append(sessionIDs, sessionID)
			}

			// Verify that the expected number of backends were created
			assert.Equal(t, tc.expectedBackends, bp.Size())

			// Stop all sessions
			for _, sessionID := range sessionIDs {
				if sessionID != "" {
					err := state.StopSession(sessionID)
					require.NoError(t, err)
				}
			}

			// Depending on the policy, there may be different numbers of backends remaining
			if tc.name == "OneToOnePolicy" || tc.name == "MaxSessionsPolicy" {
				assert.Equal(t, 0, bp.Size(), "Expected all backends to be stopped")
			}
		})
	}
}
