package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	var data = `
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
	predef := cfg.BackendProvider.Spec.(*PredefinedBackendProvider)
	assert.Len(t, predef.Endpoints, 1)
	assert.Equal(t, "http://localhost:8080", predef.Endpoints[0].Url)
}
