package grpc

import (
	"errors"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"google.golang.org/grpc/reflection"
	"reflect"
	"testing"
)

func Test_channelConfigStream_set(t *testing.T) {
	var c optVal[channelConfigStream]
	c.set(channelConfigStream{tearDown: func() error { panic(`wat`) }})
	if c.n != 1 {
		t.Error(c.n)
	}
	if c.get().tearDown == nil {
		t.Error()
	}
	c.set(channelConfigStream{})
	if c.n != 2 {
		t.Error(c.n)
	}
	if c.get().tearDown != nil {
		t.Error()
	}
}

func Test_tunnelConfig_validate(t *testing.T) {
	n := func(fn func(c *tunnelConfig)) (c tunnelConfig) {
		fn(&c)
		return
	}
	for _, tc := range [...]struct {
		Name string
		In   tunnelConfig
		Out  error
	}{
		{
			Name: `stream option required`,
			Out:  errors.New(`sesame/tun/grpc: tunnel stream option: required`),
		},
		{
			Name: `stream option twice literal`,
			In:   tunnelConfig{stream: optVal[tunnelConfigStream]{n: 2}},
			Out:  errors.New(`sesame/tun/grpc: tunnel stream option: count 2 single option required`),
		},
		{
			Name: `stream option twice options`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ClientStream(nil)(c)
				OptTunnel.ServerStream(nil)(c)
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel stream option: count 2 single option required`),
		},
		{
			Name: `nil client stream`,
			In:   n(func(c *tunnelConfig) { OptTunnel.ClientStream(nil)(c) }),
			Out:  errors.New(`sesame/tun/grpc: tunnel stream option: invalid value`),
		},
		{
			Name: `nil server stream`,
			In:   n(func(c *tunnelConfig) { OptTunnel.ServerStream(nil)(c) }),
			Out:  errors.New(`sesame/tun/grpc: tunnel stream option: invalid value`),
		},
		{
			Name: `valid server stream`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ServerStream(struct {
					grpctunnel.TunnelService_OpenTunnelServer
				}{})(c)
				if c.stream.get().stream == nil {
					t.Error()
				}
			}),
		},
		{
			Name: `valid client stream`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ClientStream(struct {
					grpctunnel.TunnelService_OpenReverseTunnelClient
				}{})(c)
				if c.stream.get().stream == nil {
					t.Error()
				}
			}),
		},
		{
			Name: `duplicate service 1`,
			In: n(func(c *tunnelConfig) {
				for i := 0; i < 2; i++ {
					OptTunnel.Service(func(h *HandlerMap) { grpchantesting.RegisterTestServiceServer(h, new(grpchantesting.TestServer)) })(c)
				}
				OptTunnel.ServerStream(struct {
					grpctunnel.TunnelService_OpenTunnelServer
				}{})(c)
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel handlers option: panic in registrar: service grpchantesting.TestService: handler already registered`),
		},
		{
			Name: `duplicate service 2`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.Service(func(h *HandlerMap) {
					for i := 0; i < 2; i++ {
						grpchantesting.RegisterTestServiceServer(h, new(grpchantesting.TestServer))
					}
				})(c)
				OptTunnel.ServerStream(struct {
					grpctunnel.TunnelService_OpenTunnelServer
				}{})(c)
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel handlers option: panic in registrar: service grpchantesting.TestService: handler already registered`),
		},
		{
			Name: `duplicate service 3`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ServerStream(struct {
					grpctunnel.TunnelService_OpenTunnelServer
				}{})(c)
				for i := 0; i < 10; i++ {
					OptTunnel.Service(func(h *HandlerMap) {
						for i := 0; i < 10; i++ {
							grpchantesting.RegisterTestServiceServer(h, new(grpchantesting.TestServer))
						}
					})(c)
				}
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel handlers option: panic in registrar: service grpchantesting.TestService: handler already registered`),
		},
		{
			Name: `stop signal nil`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ClientStream(struct {
					grpctunnel.TunnelService_OpenReverseTunnelClient
				}{})(c)
				OptTunnel.StopSignal(nil)(c)
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel stop option: invalid value`),
		},
		{
			Name: `stop signal twice`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ClientStream(struct {
					grpctunnel.TunnelService_OpenReverseTunnelClient
				}{})(c)
				for i := 0; i < 2; i++ {
					OptTunnel.StopSignal(make(chan struct{}))(c)
				}
			}),
			Out: errors.New(`sesame/tun/grpc: tunnel stop option: count 2 single option supported`),
		},
		{
			Name: `valid e2e`,
			In: n(func(c *tunnelConfig) {
				OptTunnel.ClientStream(struct {
					grpctunnel.TunnelService_OpenReverseTunnelClient
				}{})(c)

				var (
					hm         []*HandlerMap
					testServer grpchantesting.TestServer
				)
				OptTunnel.Service(func(h *HandlerMap) {
					reflection.Register(h)
					hm = append(hm, h)
				})(c)
				OptTunnel.Service(func(h *HandlerMap) {
					grpchantesting.RegisterTestServiceServer(h, &testServer)
					hm = append(hm, h)
				})(c)

				stop := make(chan struct{})
				OptTunnel.StopSignal(stop)(c)

				if c.stream.get().stream == nil {
					t.Error()
				}

				if len(hm) != 2 {
					t.Error(hm)
				} else if hm[0] == nil || hm[0] != hm[1] {
					t.Error(hm)
				} else if c.handlers.get().val != hm[0] {
					t.Error()
				} else if len(c.handlers.get().val.m) != 2 {
					t.Error(c.handlers.get().val.m)
				}

				if c.stop.get().ch == nil || reflect.ValueOf(c.stop.get().ch).Pointer() != reflect.ValueOf(stop).Pointer() {
					t.Error()
				}
			}),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.In.validate(); (err == nil) != (tc.Out == nil) || (err != nil && err.Error() != tc.Out.Error()) {
				t.Error(err)
			}
		})
	}
}

func Test_channelConfig_validate(t *testing.T) {
	n := func(fn func(c *channelConfig)) (c channelConfig) {
		fn(&c)
		return
	}
	for _, tc := range [...]struct {
		Name string
		In   channelConfig
		Out  error
	}{
		{
			Name: `stream option required`,
			Out:  errors.New(`sesame/tun/grpc: channel stream option: required`),
		},
		{
			Name: `stream option twice literal`,
			In:   channelConfig{stream: optVal[channelConfigStream]{n: 2}},
			Out:  errors.New(`sesame/tun/grpc: channel stream option: count 2 single option required`),
		},
		{
			Name: `stream option twice options`,
			In: n(func(c *channelConfig) {
				OptChannel.ClientStream(nil)(c)
				OptChannel.ServerStream(nil)(c)
			}),
			Out: errors.New(`sesame/tun/grpc: channel stream option: count 2 single option required`),
		},
		{
			Name: `nil client stream`,
			In:   n(func(c *channelConfig) { OptChannel.ClientStream(nil)(c) }),
			Out:  errors.New(`sesame/tun/grpc: channel stream option: invalid value`),
		},
		{
			Name: `nil server stream`,
			In:   n(func(c *channelConfig) { OptChannel.ServerStream(nil)(c) }),
			Out:  errors.New(`sesame/tun/grpc: channel stream option: invalid value`),
		},
		{
			Name: `valid server stream`,
			In: n(func(c *channelConfig) {
				OptChannel.ServerStream(struct {
					grpctunnel.TunnelService_OpenReverseTunnelServer
				}{})(c)
				if c.stream.get().stream == nil {
					t.Error()
				}
				if c.stream.get().tearDown != nil {
					t.Error()
				}
			}),
		},
		{
			Name: `valid client stream`,
			In: n(func(c *channelConfig) {
				OptChannel.ClientStream(struct {
					grpctunnel.TunnelService_OpenTunnelClient
				}{})(c)
				if c.stream.get().stream == nil {
					t.Error()
				}
				if c.stream.get().tearDown == nil {
					t.Error()
				}
			}),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.In.validate(); (err == nil) != (tc.Out == nil) || (err != nil && err.Error() != tc.Out.Error()) {
				t.Error(err)
			}
		})
	}
}
