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
	"sync"

	"github.com/grundprinzip/spark-connect-proxy/connect"

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

	backendProvider connect.BackendProvider

	loadPolicy connect.LoadPolicy

	// Protects sessionBackends and knownBackends
	mu sync.RWMutex
}

// StartSession assigns a backend to a new session and returns the session ID. The
// backend for the session is mapped randomly across all available backends.
func (p *ProxyState) StartSession() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionID := uuid.NewString()
	backend, err := p.loadPolicy.Next()
	if err != nil {
		return "", errors.WithType(err, errors.ProxySessionError)
	}
	p.sessionAssignments[sessionID] = backend.ID()
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

func (p *ProxyState) GetBackendForSession(sessionID string) (proxy.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if backendID, ok := p.sessionAssignments[sessionID]; ok {
		be, err := p.backendProvider.Get(backendID)
		if err != nil {
			return nil, err
		}
		return be.Connection()
	}
	return nil, errors.WithType(
		fmt.Errorf("session %s not found", sessionID),
		errors.ProxySessionError)
}

// NewProxyState creates a new proxy state.
func NewProxyState(bp connect.BackendProvider) *ProxyState {
	return &ProxyState{
		sessionAssignments: make(map[string]string),
		backendProvider:    bp,
		loadPolicy:         &RoundRobinPolicy{bp: bp},
	}
}

type RoundRobinPolicy struct {
	bp      connect.BackendProvider
	current int
}

func (p *RoundRobinPolicy) Next() (connect.Backend, error) {
	nextIndex := (p.current + 1) % p.bp.Size()
	p.current = nextIndex
	// TODO expensive call, build index
	backends, err := p.bp.List()
	if err != nil {
		return nil, err
	}
	return backends[nextIndex], nil
}
