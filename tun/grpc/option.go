package grpc

import (
	"fmt"
	"github.com/fullstorydev/grpchan"
	"github.com/joeycumines/sesame/genproto/type/grpctunnel"
	"google.golang.org/grpc"
)

const (
	_ optValCountConstraint = iota
	optValSingleRequired
	optValSingleOptional
	optValMultipleOptional
)

type (
	// ChannelOption is an option that may be provided to NewChannel.
	ChannelOption func(c *channelConfig)

	channelConfig struct {
		stream optVal[channelConfigStream]
	}
	channelConfigStream struct {
		stream   tunnelStreamClient
		tearDown func() error
	}

	// ChannelOptions exposes ChannelOption implementations as methods, which are available via the OptChannel
	// package variable.
	// TODO consider adding StopSignal option (for graceful close support)
	ChannelOptions struct{}

	// TunnelOption is an option that may be provided to ServeTunnel.
	TunnelOption func(c *tunnelConfig)

	tunnelConfig struct {
		stream   optVal[tunnelConfigStream]
		handlers optVal[tunnelConfigHandlers]
		stop     optVal[tunnelConfigStop]
	}
	tunnelConfigStream struct {
		stream tunnelStreamServer
	}
	tunnelConfigHandlers struct {
		val *HandlerMap
	}
	tunnelConfigStop struct {
		ch <-chan struct{}
	}

	// TunnelOptions exposes TunnelOption implementations as methods, which are available via the OptTunnel
	// package variable.
	TunnelOptions struct{}

	optVal[T optValI] struct {
		n   int
		opt T
	}
	optValI interface {
		name() string
		count() optValCountConstraint
		validate() error
	}
	optValCountConstraint int

	// HandlerMap implements grpc.ServiceRegistrar, and may be provided to reflection.Register.
	// WARNING this type is only intended for use by HandlerMapConfig.
	HandlerMap struct {
		m   grpchan.HandlerMap
		err error
	}

	// HandlerMapConfig models service/handler/server configuration.
	HandlerMapConfig func(h *HandlerMap)
)

var (
	// OptChannel exposes all the options for NewChannel, available as methods.
	OptChannel ChannelOptions

	// OptTunnel exposes all the options for ServeTunnel, available as methods.
	OptTunnel TunnelOptions

	// compile time assertions

	_ optValI = channelConfigStream{}
	_ optValI = tunnelConfigStream{}
	_ optValI = tunnelConfigHandlers{}
	_ optValI = tunnelConfigStop{}

	_ grpc.ServiceRegistrar = (*HandlerMap)(nil)
)

// ClientStream configures the channel to use a client-side stream.
//
// This option group (ClientStream, ServerStream) must be provided exactly once.
// This method may be accessed via the OptChannel package variable.
func (ChannelOptions) ClientStream(stream grpctunnel.TunnelService_OpenTunnelClient) ChannelOption {
	return func(c *channelConfig) {
		var v channelConfigStream
		if stream != nil {
			v = channelConfigStream{
				stream:   stream,
				tearDown: stream.CloseSend,
			}
		}
		c.stream.set(v)
	}
}

// ServerStream configures the channel to use a server-side stream.
//
// This option group (ClientStream, ServerStream) must be provided exactly once.
// This method may be accessed via the OptChannel package variable.
func (ChannelOptions) ServerStream(stream grpctunnel.TunnelService_OpenReverseTunnelServer) ChannelOption {
	return func(c *channelConfig) { c.stream.set(channelConfigStream{stream: stream}) }
}

// ClientStream configures the tunnel to use a client-side stream.
//
// This option group (ClientStream, ServerStream) must be provided exactly once.
// This method may be accessed via the OptTunnel package variable.
func (TunnelOptions) ClientStream(stream grpctunnel.TunnelService_OpenReverseTunnelClient) TunnelOption {
	return func(c *tunnelConfig) { c.stream.set(tunnelConfigStream{stream: stream}) }
}

// ServerStream configures the channel to use a server-side stream.
//
// This option group (ClientStream, ServerStream) must be provided exactly once.
// This method may be accessed via the OptTunnel package variable.
func (TunnelOptions) ServerStream(stream grpctunnel.TunnelService_OpenTunnelServer) TunnelOption {
	return func(c *tunnelConfig) { c.stream.set(tunnelConfigStream{stream: stream}) }
}

// Service registers 0-n services into the internal handler, additive with any existing services.
// Duplicate handlers (for the same service) will result in an error.
// See also HandlerMap and HandlerMapConfig.
//
// This option may be provided multiple times, and is optional.
// This method may be accessed via the OptTunnel package variable.
func (TunnelOptions) Service(config HandlerMapConfig) TunnelOption {
	return func(c *tunnelConfig) { c.handlers.set(c.handlers.get().merge(config)) }
}

// StopSignal configures a channel that should be closed to indicate that a (graceful) stop has been initiated.
// A nil value will result in an error.
//
// This option may be provided at most once.
// This method may be accessed via the OptTunnel package variable.
func (TunnelOptions) StopSignal(ch <-chan struct{}) TunnelOption {
	return func(c *tunnelConfig) { c.stop.set(tunnelConfigStop{ch: ch}) }
}

func (x channelConfig) validate() (err error) {
	if err = x.stream.validate(); err != nil {
		return
	}
	return
}

func (x tunnelConfig) validate() (err error) {
	if err = x.stream.validate(); err != nil {
		return
	}
	if err = x.handlers.validate(); err != nil {
		return
	}
	if err = x.stop.validate(); err != nil {
		return
	}
	return
}

func (x channelConfigStream) name() string                 { return `channel stream` }
func (x channelConfigStream) count() optValCountConstraint { return optValSingleRequired }
func (x channelConfigStream) validate() error {
	if x.stream == nil {
		return x.errorf(`invalid value`)
	}
	return nil
}
func (x channelConfigStream) errorf(format string, args ...interface{}) error {
	return optErrorf(x.name(), format, args...)
}

func (x tunnelConfigStop) name() string                 { return `tunnel stop` }
func (x tunnelConfigStop) count() optValCountConstraint { return optValSingleOptional }
func (x tunnelConfigStop) validate() error {
	if x.ch == nil {
		return x.errorf(`invalid value`)
	}
	return nil
}
func (x tunnelConfigStop) errorf(format string, args ...interface{}) error {
	return optErrorf(x.name(), format, args...)
}

func (x tunnelConfigHandlers) name() string                 { return `tunnel handlers` }
func (x tunnelConfigHandlers) count() optValCountConstraint { return optValMultipleOptional }
func (x tunnelConfigHandlers) validate() error {
	if x.val != nil && x.val.err != nil {
		return x.errorf(`%s`, x.val.err)
	}
	return nil
}
func (x tunnelConfigHandlers) errorf(format string, args ...interface{}) error {
	return optErrorf(x.name(), format, args...)
}
func (x tunnelConfigHandlers) merge(config HandlerMapConfig) tunnelConfigHandlers {
	if x.val == nil {
		x.val = new(HandlerMap)
	}
	if x.val.err == nil {
		config(x.val)
	}
	return x
}

func (x tunnelConfigStream) name() string                 { return `tunnel stream` }
func (x tunnelConfigStream) count() optValCountConstraint { return optValSingleRequired }
func (x tunnelConfigStream) validate() error {
	if x.stream == nil {
		return x.errorf(`invalid value`)
	}
	return nil
}
func (x tunnelConfigStream) errorf(format string, args ...interface{}) error {
	return optErrorf(x.name(), format, args...)
}

func (x *optVal[T]) set(opt T) {
	x.n++
	x.opt = opt
}
func (x *optVal[T]) get() T { return x.opt }
func (x optVal[T]) validate() error {
	switch x.opt.count() {
	case optValSingleRequired:
		switch x.n {
		case 0:
			return x.errorf(`required`)
		case 1:
		default:
			return x.errorf(`count %d single option required`, x.n)
		}
	case optValSingleOptional:
		switch x.n {
		case 0:
		case 1:
		default:
			return x.errorf(`count %d single option supported`, x.n)
		}
	case optValMultipleOptional:
	default:
		panic(x.errorf(`unknown opt val count constraint %d`, x.opt.count()))
	}
	if x.n != 0 {
		return x.opt.validate()
	}
	return nil
}
func (x optVal[T]) errorf(format string, args ...interface{}) error {
	return optErrorf(x.opt.name(), format, args...)
}

func optErrorf(name string, format string, args ...interface{}) error {
	a := make([]interface{}, 0, len(args)+1)
	a = append(a, name)
	a = append(a, args...)
	return fmt.Errorf(`sesame/tun/grpc: %s option: `+format, a...)
}

func (x *HandlerMap) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	if x.err != nil {
		return
	}
	if x.m == nil {
		x.m = make(grpchan.HandlerMap)
	}
	var success bool
	defer func() {
		if !success {
			x.err = fmt.Errorf(`panic in registrar: %v`, recover())
		}
	}()
	x.m.RegisterService(desc, impl)
	success = true
}

func (x *HandlerMap) GetServiceInfo() map[string]grpc.ServiceInfo {
	return x.m.GetServiceInfo()
}

func (x *HandlerMap) grpchan() grpchan.HandlerMap {
	if x != nil {
		return x.m
	}
	return nil
}
