//go:build tools
// +build tools

package tools

import (
	_ "github.com/joeycumines/protoc-gen-go-copy"
	_ "github.com/joeycumines/sesame/internal/cmd/protoc-gen-sesame"
	_ "golang.org/x/tools/cmd/godoc"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
