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
	"os"

	"github.com/grundprinzip/spark-connect-proxy/connect"

	"gopkg.in/yaml.v3"
)

type BackendProvider struct {
	Name string      `yaml:"name"`
	Type string      `yaml:"type"`
	Spec interface{} `yaml:"-"`
}

type LoadPolicyConfig struct {
	Type   string            `yaml:"name"`
	Params map[string]string `yaml:"params"`
}

// TLSConfig defines the TLS/SSL configuration for secure connections
type TLSConfig struct {
	// Enabled indicates whether TLS should be used for connections
	Enabled bool `yaml:"enabled"`
	// CertFile is the path to the certificate file (PEM format)
	CertFile string `yaml:"cert_file"`
	// KeyFile is the path to the private key file (PEM format)
	KeyFile string `yaml:"key_file"`
	// ServerName is the server name to use in the TLS configuration (optional)
	// This is useful when the certificate has a different hostname than the one being used
	ServerName string `yaml:"server_name,omitempty"`
}

// ServerConfig defines the server configuration for the proxy
type ServerConfig struct {
	// ListenAddr is the address to listen on (e.g., "localhost:8080", ":8080")
	// If empty, defaults to "localhost:8080"
	ListenAddr string `yaml:"listen_addr"`
	// TLS defines the TLS configuration for secure connections
	TLS TLSConfig `yaml:"tls"`
}

// Configuration represents the complete configuration for the Spark Connect Proxy
type Configuration struct {
	// BackendProvider configures the backend connection provider
	BackendProvider BackendProvider `yaml:"backend_provider"`
	// LogLevel defines the logging level (debug, info, warn, error)
	LogLevel string `yaml:"log_level"`
	// LoadPolicy defines how to distribute sessions across backends
	LoadPolicy LoadPolicyConfig `yaml:"load_policy"`
	// Server configures the proxy server settings including TLS
	Server ServerConfig `yaml:"server"`
}

func (s *BackendProvider) UnmarshalYAML(n *yaml.Node) error {
	type S BackendProvider
	type T struct {
		*S   `yaml:",inline"`
		Spec yaml.Node `yaml:"spec"`
	}

	obj := &T{S: (*S)(s)}
	if err := n.Decode(obj); err != nil {
		return err
	}

	// We can't access the BackendTLS config here yet, so we'll store the YAML node
	// and finish initialization in LoadConfigData
	s.Spec = obj.Spec
	return nil
}

func LoadConfigData(data []byte) (*Configuration, error) {
	var config Configuration
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	// Now that we have the full config with TLS settings, we can initialize the backend provider
	if config.BackendProvider.Type == "PREDEFINED" {
		// Initialize the backend provider with TLS config
		spec, err := connect.NewPredefinedBackendProvider(
			config.BackendProvider.Spec.(yaml.Node))
		if err != nil {
			return nil, err
		}
		config.BackendProvider.Spec = spec
	}

	return &config, nil
}

// LoadConfig loads the configuration from the given file.
func LoadConfig(file string) (*Configuration, error) {
	// Open the file and read the contents to bytes
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return LoadConfigData(data)
}
