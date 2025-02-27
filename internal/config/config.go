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
	"gopkg.in/yaml.v3"
	"os"
)

type BackendProvider struct {
	Name string      `yaml:"name"`
	Type string      `yaml:"type"`
	Spec interface{} `yaml:"-"`
}

type PredefinedBackendProvider struct {
	Endpoints []struct {
		Url string `yaml:"url"`
	} `yaml:"endpoints"`
}

type Configuration struct {
	BackendProvider BackendProvider `yaml:"backend_provider"`
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

	switch s.Type {
	case "PREDEFINED":
		s.Spec = new(PredefinedBackendProvider)
	default:
		panic("type unknown")
	}
	return obj.Spec.Decode(s.Spec)
}

func LoadConfigData(data []byte) (*Configuration, error) {
	var config Configuration
	err := yaml.Unmarshal(data, &config)
	return &config, err
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
