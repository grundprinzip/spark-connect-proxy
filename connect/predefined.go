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

type PredefinedBackendProvider struct {
	Endpoints []struct {
		Url string `yaml:"url"`
	} `yaml:"endpoints"`
}

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
	provider := new(PredefinedBackendProvider)
	if err := node.Decode(provider); err != nil {
		return nil, err
	}
	return provider, nil
}

func (b *PredefinedBackendProvider) Start() (Backend, error) {
	return nil, errors.New("Cannot start backend")
}

func (b *PredefinedBackendProvider) Stop(id string) error {
	return errors.New("Cannot stop backend")
}

func (b *PredefinedBackendProvider) List() ([]Backend, error) {
	backends := make([]Backend, 0)
	for _, endpoint := range b.Endpoints {
		backends = append(backends, NewPredefinedBackend(endpoint.Url))
	}
	return backends, nil
}

func (b *PredefinedBackendProvider) Get(id string) (Backend, error) {
	for _, endpoint := range b.Endpoints {
		if endpoint.Url == id {
			return NewPredefinedBackend(endpoint.Url), nil
		}
	}
	return nil, errors.New("Backend not found")
}

func (b *PredefinedBackendProvider) Size() int {
	return len(b.Endpoints)
}
