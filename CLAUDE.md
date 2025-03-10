# Spark Connect Proxy Dev Guide

## Build & Test Commands
- Build: `make build`
- Format code: `make fmt`
- Run all tests: `make test`
- Run single test: `go test -v ./path/to/package -run TestName`
- Test with coverage: `make coverage`
- Check license/lint: `make check`

## Code Style Guidelines
- **Formatting**: Use gofumpt (enforced by golangci-lint)
- **Imports**: Standard lib first, project imports second, third-party last (grouped with blank lines)
- **Naming**:
  - PascalCase for exported functions/types (public)
  - camelCase for non-exported functions (private)
  - Lowercase package names
- **Error Handling**: Use custom error types with wrapping (go-errors/errors package)
- **Documentation**: All exported functions should have doc comments
- **Testing**: Use testify assertions, place tests in *_test.go next to implementation
- **License Headers**: Required for all files (checked by dev/check-license)