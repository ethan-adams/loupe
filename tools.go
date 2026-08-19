//go:build tools

// Package tools pins build-time-only dependencies (the gqlgen code generator)
// so `go mod tidy` keeps them and `go run github.com/99designs/gqlgen generate`
// works. It is never compiled into the binary.
package tools

import _ "github.com/99designs/gqlgen"
