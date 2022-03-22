// Copyright 2018 Joshua Humphries
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package grpc provides tools to support tunneling of gRPC services:
// carrying gRPC calls over a gRPC stream.
//
// This support includes "pinning" an RPC channel to a single server, by sending
// all requests on a single gRPC stream. There are also tools for adapting
// certain kinds of bidirectional stream RPCs into a stub such that a single
// stream looks like a sequence of unary calls.
//
// This support also includes "reverse services", where a client can initiate a
// connection to a server and subsequently the server can then wrap that
// connection with an RPC stub, used to send requests from the server to that
// client (and the client then replies and sends responses back to the server).
package grpc
