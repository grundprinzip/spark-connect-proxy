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
	"maps"
	"math/rand"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/grundprinzip/spark-connect-proxy/internal/errors"
	"github.com/siderolabs/grpc-proxy/proxy"
)

// ProxyState represents the state of the proxy.
// It captures the list of konwn backends and the mapping of sessions to backends.
// These members are protected by a mutex to make the state thread-safe.
//
// To register a new session in the state, the caller has to call StartSession.
// Removing a session from the state is done by calling StopSession.
//
// Backends can always be added, but they can only be removed when no sessions
// are assigned to them.
type ProxyState struct {
	// A map of session IDs to backend IDs.
	sessionAssignments map[string]string

	// A map of backend IDs to backend objects.
	backends map[string]proxy.Backend

	// Protects sessionBackends and knownBackends
	mu sync.RWMutex
}

// AddBackend assigns a backend to a backend ID and registers it in the proxy state.
func (p *ProxyState) AddBackend(backendID string, backend proxy.Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends[backendID] = backend
}

// RemoveBackend removes a backend from the proxy state. The backend can only be removed
// if no sessions are assigned to it.
func (p *ProxyState) RemoveBackend(backendID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.backends[backendID]; !ok {
		return errors.WithType(
			fmt.Errorf("backend %s not found", backendID),
			errors.ProxySessionError)
	}

	// Check if any sessions are assigned to the backend.
	for _, b := range p.sessionAssignments {
		if b == backendID {
			return errors.WithType(
				fmt.Errorf("backend %s is still assigned to a session", backendID),
				errors.ProxyInitError)
		}
	}

	delete(p.backends, backendID)
	return nil
}

// StartSession assigns a backend to a new session and returns the session ID. The
// backend for the session is mapped randomly across all available backends.
func (p *ProxyState) StartSession() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionID := uuid.NewString()
	backendID, err := p.randomBackend()
	if err != nil {
		return "", errors.WithType(err, errors.ProxySessionError)
	}
	p.sessionAssignments[sessionID] = backendID
	return sessionID, nil
}

// StopSession removes a session from the proxy state.
func (p *ProxyState) StopSession(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.sessionAssignments[id]; !ok {
		return errors.WithType(fmt.Errorf("session %s not found", id), errors.ProxySessionError)
	}

	delete(p.sessionAssignments, id)
	return nil
}

// randomBackend returns a random backend ID from the list of
// known backends.
func (p *ProxyState) randomBackend() (string, error) {
	numBackends := len(p.backends)
	if numBackends == 0 {
		return "", errors.WithType(
			fmt.Errorf("no backends available"),
			errors.ProxyInitError)
	}
	keys := slices.Collect(maps.Keys(p.backends))
	return keys[rand.Intn(numBackends)], nil
}

func (p *ProxyState) GetBackendForSession(sessionID string) (proxy.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if backendID, ok := p.sessionAssignments[sessionID]; ok {
		if backend, ok := p.backends[backendID]; ok {
			return backend, nil
		}
		return nil, errors.WithType(
			fmt.Errorf("backend %s not found", backendID),
			errors.ProxySessionError)
	}

	return nil, errors.WithType(
		fmt.Errorf("session %s not found", sessionID),
		errors.ProxySessionError)
}

// NewProxyState creates a new proxy state.
func NewProxyState() *ProxyState {
	return &ProxyState{
		sessionAssignments: make(map[string]string),
		backends:           make(map[string]proxy.Backend),
	}
}
