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

	// Logger for proxy operations
	logger *slog.Logger

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

	backend, ok := p.sessionAssignments[id]
	if !ok {
		return errors.WithType(fmt.Errorf("session %s not found", id), errors.ProxySessionError)
	}
	// Stop the backend in the backend provider.
	if err := p.loadPolicy.Release(backend); err != nil {
		return errors.WithType(err, errors.ProxySessionError)
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
func NewProxyState(bp connect.BackendProvider, loadPolicy connect.LoadPolicy, logger *slog.Logger) *ProxyState {
	return &ProxyState{
		sessionAssignments: make(map[string]string),
		backendProvider:    bp,
		loadPolicy:         loadPolicy,
		logger:             logger,
	}
}

// SetLogger sets the logger for the proxy state and propagates it to the backend provider and load policy
func (p *ProxyState) SetLogger(logger *slog.Logger) {
	p.logger = logger
	if p.backendProvider != nil {
		p.backendProvider.SetLogger(logger.With("component", "backend_provider"))
	}
	if p.loadPolicy != nil {
		p.loadPolicy.SetLogger(logger.With("component", "load_policy"))
	}
}

// RoundRobinPolicy is a policy that selects backends in a round-robin fashion.
type RoundRobinPolicy struct {
	bp      connect.BackendProvider
	current int
	logger  *slog.Logger
}

func (p *RoundRobinPolicy) Release(id string) error {
	return nil
}

func (p *RoundRobinPolicy) Next() (connect.Backend, error) {
	// If there are no backends registered, try to start at least one.
	if p.bp.Size() == 0 {
		if p.logger != nil {
			p.logger.Debug("No backends registered, starting a new one")
		}
		return p.bp.Start()
	}

	nextIndex := (p.current + 1) % p.bp.Size()
	p.current = nextIndex
	// TODO expensive call, build index
	backends, err := p.bp.List()
	if err != nil {
		return nil, err
	}

	if p.logger != nil {
		p.logger.Debug("Selected backend in round-robin",
			"index", nextIndex,
			"backend_id", backends[nextIndex].ID())
	}
	return backends[nextIndex], nil
}

func (p *RoundRobinPolicy) SetLogger(logger *slog.Logger) {
	p.logger = logger
	if p.logger != nil {
		p.logger.Info("Logger set for RoundRobinPolicy")
	}
}

// OneToOnePolicy is a policy that assigns a single backend to each session.
type OneToOnePolicy struct {
	bp     connect.BackendProvider
	logger *slog.Logger
}

func (p *OneToOnePolicy) Next() (connect.Backend, error) {
	if p.logger != nil {
		p.logger.Debug("Starting new backend for session")
	}
	backend, err := p.bp.Start()
	if err == nil && p.logger != nil {
		p.logger.Debug("Started new backend", "backend_id", backend.ID())
	}
	return backend, err
}

func (p *OneToOnePolicy) Release(id string) error {
	if p.logger != nil {
		p.logger.Debug("Stopping backend", "backend_id", id)
	}
	return p.bp.Stop(id)
}

func (p *OneToOnePolicy) SetLogger(logger *slog.Logger) {
	p.logger = logger
	if p.logger != nil {
		p.logger.Info("Logger set for OneToOnePolicy")
	}
}

// MaxSessionsPolicy is a policy that limits the number of sessions that can be
// associated with a single backend. For that reason the policy has to track how
// many new sessions are associated with a single backend.
type MaxSessionsPolicy struct {
	bp connect.BackendProvider
	// Maximum number of sessions per backend.
	maxSize int
	// Mapping of backend to number of sessions.
	sessions map[string]int
	// Logger for policy operations
	logger *slog.Logger
}

func (p *MaxSessionsPolicy) Next() (connect.Backend, error) {
	// If there are no backends registered, try to start at least one.
	if p.bp.Size() == 0 {
		if p.logger != nil {
			p.logger.Debug("No backends registered, starting a new one")
		}
		be, err := p.bp.Start()
		if err != nil {
			return nil, err
		}
		p.sessions[be.ID()] = 1
		if p.logger != nil {
			p.logger.Debug("Started new backend", "backend_id", be.ID(), "sessions", 1)
		}
		return be, nil
	}

	// Find the backend with the fewest sessions.
	backends, err := p.bp.List()
	if err != nil {
		return nil, err
	}
	minSessions := p.sessions[backends[0].ID()]
	minIndex := 0
	for i := 1; i < len(backends); i++ {
		if p.sessions[backends[i].ID()] < minSessions {
			minSessions = p.sessions[backends[i].ID()]
			minIndex = i
		}
	}

	// If the minimum number of sessions is less than the maximum size, return the backend.
	// Otherwise, create a new backend.
	if minSessions < p.maxSize {
		backendID := backends[minIndex].ID()
		p.sessions[backendID]++
		if p.logger != nil {
			p.logger.Debug("Using existing backend",
				"backend_id", backendID,
				"sessions", p.sessions[backendID])
		}
		return backends[minIndex], nil
	} else {
		if p.logger != nil {
			p.logger.Debug("All backends at max capacity, starting a new one",
				"max_sessions", p.maxSize)
		}
		be, err := p.bp.Start()
		if err != nil {
			return nil, err
		}
		p.sessions[be.ID()] = 1
		if p.logger != nil {
			p.logger.Debug("Started new backend", "backend_id", be.ID(), "sessions", 1)
		}
		return be, nil
	}
}

func (p *MaxSessionsPolicy) Release(id string) error {
	if _, ok := p.sessions[id]; !ok {
		if p.logger != nil {
			p.logger.Warn("Attempted to release unknown backend", "backend_id", id)
		}
		return fmt.Errorf("backend %s not found", id)
	}

	// Check if the backend has only one session left, then stop the backend.
	if p.sessions[id] == 1 {
		if p.logger != nil {
			p.logger.Debug("Stopping backend with last session", "backend_id", id)
		}
		if err := p.bp.Stop(id); err != nil {
			return err
		}
		delete(p.sessions, id)
		return nil
	}

	p.sessions[id]--
	if p.logger != nil {
		p.logger.Debug("Released session from backend",
			"backend_id", id,
			"remaining_sessions", p.sessions[id])
	}
	return nil
}

func (p *MaxSessionsPolicy) SetLogger(logger *slog.Logger) {
	p.logger = logger
	if p.logger != nil {
		p.logger.Info("Logger set for MaxSessionsPolicy", "max_sessions", p.maxSize)
	}
}
