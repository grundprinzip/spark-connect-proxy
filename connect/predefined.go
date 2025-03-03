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
	"errors"

	"github.com/siderolabs/grpc-proxy/proxy"
	"gopkg.in/yaml.v3"
)

// PredefinedBackendProviderConfig is the configuration for the predefined backend provider
// it contains a list of endpoints. Later this configuration is translated into the actual
// implementation of the backend provider interface.
type PredefinedBackendProviderConfig struct {
	Endpoints []struct {
		Url string `yaml:"url"`
	} `yaml:"endpoints"`
}

// PredefinedBackendProvider is a backend provider that provides a list of predefined backends
// and implements the BackendProvider interface.
type PredefinedBackendProvider struct {
	Endpoints []Backend
}

// PredefinedBackend is a backend that is predefined and implements the Backend interface.
type PredefinedBackend struct {
	url string
}

func NewPredefinedBackend(url string) *PredefinedBackend {
	return &PredefinedBackend{url: url}
}

func (b *PredefinedBackend) ID() string {
	return b.url
}

func (b *PredefinedBackend) Url() string {
	return b.url
}

func (b *PredefinedBackend) Connection() (proxy.Backend, error) {
	// TODO: cache this
	return CreateSimpleProxyBackend(b.url)
}

func NewPredefinedBackendProvider(node yaml.Node) (*PredefinedBackendProvider, error) {
	provider := new(PredefinedBackendProviderConfig)
	if err := node.Decode(provider); err != nil {
		return nil, err
	}

	// Convert the endpoints to backends
	backends := make([]Backend, 0)
	for _, endpoint := range provider.Endpoints {
		backends = append(backends, NewPredefinedBackend(endpoint.Url))
	}

	// Return the actual backend provider
	return &PredefinedBackendProvider{
		Endpoints: backends,
	}, nil
}

func (b *PredefinedBackendProvider) Start() (Backend, error) {
	return nil, errors.New("Cannot start backend")
}

func (b *PredefinedBackendProvider) Stop(id string) error {
	return errors.New("Cannot stop backend")
}

func (b *PredefinedBackendProvider) List() ([]Backend, error) {
	return b.Endpoints, nil
}

func (b *PredefinedBackendProvider) Get(id string) (Backend, error) {
	for _, endpoint := range b.Endpoints {
		if endpoint.ID() == id {
			return endpoint, nil
		}
	}
	return nil, errors.New("Backend not found")
}

func (b *PredefinedBackendProvider) Size() int {
	return len(b.Endpoints)
}
