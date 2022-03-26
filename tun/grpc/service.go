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

package grpc

import (
	"github.com/fullstorydev/grpchan"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
)

// TunnelServer provides an implementation for grpctunnel.TunnelServiceServer.
// You can register handlers with it, and it will then expose those handlers
// for incoming tunnels. If no handlers are registered, the server will reply
// to OpenTunnel requests with an "Unimplemented" error code. The server may
// still be used for reverse tunnels
//
// For reverse tunnels, if supported, all connected channels (e.g. all clients
// that have created reverse tunnels) are available. You can also configure a
// listener to receive notices when channels are connected and disconnected.
type TunnelServer struct {
	// If set, reverse tunnels will not be allowed. The server will reply to
	// OpenReverseTunnel requests with an "Unimplemented" error code.
	NoReverseTunnels bool
	// If reverse tunnels are allowed, this callback may be configured to
	// receive information when clients open a reverse tunnel.
	OnReverseTunnelConnect func(channel *Channel)
	// If reverse tunnels are allowed, this callback may be configured to
	// receive information when reverse tunnels are torn down.
	OnReverseTunnelDisconnect func(channel *Channel)
	// Optional function that accepts a reverse tunnel and returns an affinity
	// key. The affinity key values can be used to look up outbound channels,
	// for targeting calls to particular clients or groups of clients.
	AffinityKey func(channel *Channel) interface{}
	// Optional channel to signal graceful close. The channel should be closed
	// to indicate (graceful) stop has been started. Must be set prior to
	// using the server.
	StopSignal <-chan struct{}

	// TODO consider exposing mechanism to resolve additional options for NewChannel and ServeTunnel

	handlers grpchan.HandlerMap

	reverse reverseChannels

	mu           sync.RWMutex
	reverseByKey map[interface{}]*reverseChannels

	unimplementedTunnelServiceServer
}

type unimplementedTunnelServiceServer = UnimplementedTunnelServiceServer

var _ TunnelServiceServer = (*TunnelServer)(nil)
var _ grpc.ServiceRegistrar = (*TunnelServer)(nil)

func (s *TunnelServer) RegisterService(desc *grpc.ServiceDesc, srv interface{}) {
	if s.handlers == nil {
		s.handlers = grpchan.HandlerMap{}
	}
	s.handlers.RegisterService(desc, srv)
}

func (s *TunnelServer) GetServiceInfo() map[string]grpc.ServiceInfo {
	return s.handlers.GetServiceInfo()
}

func (s *TunnelServer) OpenTunnel(stream TunnelService_OpenTunnelServer) error {
	if len(s.handlers) == 0 {
		return status.Error(codes.Unimplemented, "forward tunnels not supported")
	}
	options := []TunnelOption{
		OptTunnel.ServerStream(stream),
		OptTunnel.Service(func(h *HandlerMap) { s.handlers.ForEach(h.RegisterService) }),
	}
	if s.StopSignal != nil {
		options = append(options, OptTunnel.StopSignal(s.StopSignal))
	}
	return ServeTunnel(options...)
}

func (s *TunnelServer) OpenReverseTunnel(stream TunnelService_OpenReverseTunnelServer) error {
	if s.NoReverseTunnels {
		return status.Error(codes.Unimplemented, "reverse tunnels not supported")
	}

	// TODO stop signal support for the reverse tunnel side
	ch, err := NewChannel(OptChannel.ServerStream(stream))
	if err != nil {
		return err
	}
	defer ch.Close()

	var key interface{}
	if s.AffinityKey != nil {
		key = s.AffinityKey(ch)
	}

	s.reverse.add(ch)
	defer s.reverse.remove(ch)

	rc := func() *reverseChannels {
		s.mu.Lock()
		defer s.mu.Unlock()

		rc := s.reverseByKey[key]
		if rc == nil {
			rc = &reverseChannels{}
			if s.reverseByKey == nil {
				s.reverseByKey = map[interface{}]*reverseChannels{}
			}
			s.reverseByKey[key] = rc
		}
		return rc
	}()
	rc.add(ch)
	defer rc.remove(ch)

	if s.OnReverseTunnelConnect != nil {
		s.OnReverseTunnelConnect(ch)
	}
	if s.OnReverseTunnelDisconnect != nil {
		defer s.OnReverseTunnelDisconnect(ch)
	}

	<-ch.Done()
	return ch.Err()
}

type reverseChannels struct {
	mu    sync.Mutex
	chans []*Channel
	idx   int
}

func (c *reverseChannels) allChans() []*Channel {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]*Channel, len(c.chans))
	copy(cp, c.chans)
	return cp
}

func (c *reverseChannels) pick() grpc.ClientConnInterface {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.chans) == 0 {
		return nil
	}

	c.idx++
	if c.idx >= len(c.chans) {
		c.idx = 0
	}

	return c.chans[c.idx]
}

func (c *reverseChannels) add(ch *Channel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.chans = append(c.chans, ch)
}

func (c *reverseChannels) remove(ch *Channel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.chans {
		if c.chans[i] == ch {
			c.chans = append(c.chans[:i], c.chans[i+1:]...)
			break
		}
	}
}

func (s *TunnelServer) AllReverseTunnels() []*Channel {
	return s.reverse.allChans()
}

func (s *TunnelServer) AsChannel() grpc.ClientConnInterface {
	if s.NoReverseTunnels {
		panic("reverse tunnels not supported")
	}
	return multiChannel(s.reverse.pick)
}

func (s *TunnelServer) KeyAsChannel(key interface{}) grpc.ClientConnInterface {
	if s.NoReverseTunnels {
		panic("reverse tunnels not supported")
	}
	return multiChannel(func() grpc.ClientConnInterface { return s.pickKey(key) })
}

func (s *TunnelServer) FindChannel(search func(*Channel) bool) *Channel {
	if s.NoReverseTunnels {
		panic("reverse tunnels not supported")
	}
	allChans := s.reverse.allChans()
	for _, ch := range allChans {
		if !ch.Canceled() && search(ch) {
			return ch
		}
	}
	return nil
}

func (s *TunnelServer) pickKey(key interface{}) grpc.ClientConnInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reverseByKey[key].pick()
}

type multiChannel func() grpc.ClientConnInterface

func (c multiChannel) Invoke(ctx context.Context, methodName string, req, resp interface{}, opts ...grpc.CallOption) error {
	ch := c()
	if ch == nil {
		return status.Errorf(codes.Unavailable, "no channels ready")
	}
	return ch.Invoke(ctx, methodName, req, resp, opts...)
}

func (c multiChannel) NewStream(ctx context.Context, desc *grpc.StreamDesc, methodName string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ch := c()
	if ch == nil {
		return nil, status.Errorf(codes.Unavailable, "no channels ready")
	}
	return ch.NewStream(ctx, desc, methodName, opts...)
}
