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

package config

import (
	"testing"

	"github.com/grundprinzip/spark-connect-proxy/connect"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	data := `
backend_provider:
  name: manual spark
  type: PREDEFINED
  spec:
    endpoints:
      - url: http://localhost:8080
`
	cfg, err := LoadConfigData([]byte(data))
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "manual spark", cfg.BackendProvider.Name)
	assert.Equal(t, "PREDEFINED", cfg.BackendProvider.Type)
	predef := cfg.BackendProvider.Spec.(*connect.PredefinedBackendProvider)
	assert.Len(t, predef.Endpoints, 1)
	assert.Equal(t, "http://localhost:8080", predef.Endpoints[0].Url())
}

func TestLoadConfigWithTLS(t *testing.T) {
	data := `
backend_provider:
  name: manual spark
  type: PREDEFINED
  spec:
    endpoints:
      - url: http://localhost:8080
server:
  listen_addr: "localhost:9090"
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
    server_name: "test.example.com"
`
	cfg, err := LoadConfigData([]byte(data))
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check TLS configuration
	assert.Equal(t, "localhost:9090", cfg.Server.ListenAddr)
	assert.True(t, cfg.Server.TLS.Enabled)
	assert.Equal(t, "/path/to/cert.pem", cfg.Server.TLS.CertFile)
	assert.Equal(t, "/path/to/key.pem", cfg.Server.TLS.KeyFile)
	assert.Equal(t, "test.example.com", cfg.Server.TLS.ServerName)
}

func TestLoadConfigWithoutTLS(t *testing.T) {
	data := `
backend_provider:
  name: manual spark
  type: PREDEFINED
  spec:
    endpoints:
      - url: http://localhost:8080
server:
  listen_addr: "localhost:9090"
  tls:
    enabled: false
`
	cfg, err := LoadConfigData([]byte(data))
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check server configuration
	assert.Equal(t, "localhost:9090", cfg.Server.ListenAddr)
	assert.False(t, cfg.Server.TLS.Enabled)
	assert.Empty(t, cfg.Server.TLS.CertFile)
	assert.Empty(t, cfg.Server.TLS.KeyFile)
	assert.Empty(t, cfg.Server.TLS.ServerName)
}
