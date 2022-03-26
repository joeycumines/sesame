// Code generated by protoc-gen-go-copy. DO NOT EDIT.
// source: sesame/v1alpha1/tunnel.proto

package tun

import "github.com/joeycumines/sesame/tun/grpc"
import "google.golang.org/genproto/googleapis/rpc/status"

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *TunnelRequest:
		x.Id = v.GetId()
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface{ GetId() uint64 }); ok {
			x.Id = v.GetId()
		}
		if v, ok := v.(interface{ GetData() isTunnelRequest_Data }); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface{ GetConnect() *TunnelRequest_Connect }); ok {
					var defaultValue *TunnelRequest_Connect
					if v := v.GetConnect(); v != defaultValue {
						x.Data = &TunnelRequest_Connect_{Connect: v}
						return
					}
				}
				if v, ok := v.(interface{ GetStream() *TunnelRequest_Stream }); ok {
					var defaultValue *TunnelRequest_Stream
					if v := v.GetStream(); v != defaultValue {
						x.Data = &TunnelRequest_Stream_{Stream: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelRequest) Proto_ShallowClone() (c *TunnelRequest) {
	if x != nil {
		c = new(TunnelRequest)
		c.Id = x.Id
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelRequest_Connect) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *TunnelRequest_Connect:
		x.Remote = v.GetRemote()
	default:
		if v, ok := v.(interface{ GetRemote() string }); ok {
			x.Remote = v.GetRemote()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelRequest_Connect) Proto_ShallowClone() (c *TunnelRequest_Connect) {
	if x != nil {
		c = new(TunnelRequest_Connect)
		c.Remote = x.Remote
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelRequest_Stream) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *TunnelRequest_Stream:
		x.Error = v.GetError()
	default:
		if v, ok := v.(interface{ GetError() *status.Status }); ok {
			x.Error = v.GetError()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelRequest_Stream) Proto_ShallowClone() (c *TunnelRequest_Stream) {
	if x != nil {
		c = new(TunnelRequest_Stream)
		c.Error = x.Error
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelResponse) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *TunnelResponse:
		x.Id = v.GetId()
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface{ GetId() uint64 }); ok {
			x.Id = v.GetId()
		}
		if v, ok := v.(interface{ GetData() isTunnelResponse_Data }); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface {
					GetConnect() *TunnelResponse_Connect
				}); ok {
					var defaultValue *TunnelResponse_Connect
					if v := v.GetConnect(); v != defaultValue {
						x.Data = &TunnelResponse_Connect_{Connect: v}
						return
					}
				}
				if v, ok := v.(interface{ GetStream() *TunnelResponse_Stream }); ok {
					var defaultValue *TunnelResponse_Stream
					if v := v.GetStream(); v != defaultValue {
						x.Data = &TunnelResponse_Stream_{Stream: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelResponse) Proto_ShallowClone() (c *TunnelResponse) {
	if x != nil {
		c = new(TunnelResponse)
		c.Id = x.Id
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelResponse_Connect) Proto_ShallowCopy(v interface{}) {
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelResponse_Connect) Proto_ShallowClone() (c *TunnelResponse_Connect) {
	if x != nil {
		c = new(TunnelResponse_Connect)
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *TunnelResponse_Stream) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *TunnelResponse_Stream:
		x.Endpoint = v.GetEndpoint()
	default:
		if v, ok := v.(interface{ GetEndpoint() string }); ok {
			x.Endpoint = v.GetEndpoint()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *TunnelResponse_Stream) Proto_ShallowClone() (c *TunnelResponse_Stream) {
	if x != nil {
		c = new(TunnelResponse_Stream)
		c.Endpoint = x.Endpoint
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *StreamRequest:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface{ GetData() isStreamRequest_Data }); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface{ GetConnect() *StreamRequest_Connect }); ok {
					var defaultValue *StreamRequest_Connect
					if v := v.GetConnect(); v != defaultValue {
						x.Data = &StreamRequest_Connect_{Connect: v}
						return
					}
				}
				if v, ok := v.(interface{ GetDial() *StreamRequest_Dial }); ok {
					var defaultValue *StreamRequest_Dial
					if v := v.GetDial(); v != defaultValue {
						x.Data = &StreamRequest_Dial_{Dial: v}
						return
					}
				}
				if v, ok := v.(interface{ GetTunnel() *grpc.ServerToClient }); ok {
					var defaultValue *grpc.ServerToClient
					if v := v.GetTunnel(); v != defaultValue {
						x.Data = &StreamRequest_Tunnel{Tunnel: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamRequest) Proto_ShallowClone() (c *StreamRequest) {
	if x != nil {
		c = new(StreamRequest)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamRequest_Connect) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *StreamRequest_Connect:
		x.Remote = v.GetRemote()
	default:
		if v, ok := v.(interface{ GetRemote() string }); ok {
			x.Remote = v.GetRemote()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamRequest_Connect) Proto_ShallowClone() (c *StreamRequest_Connect) {
	if x != nil {
		c = new(StreamRequest_Connect)
		c.Remote = x.Remote
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamRequest_Dial) Proto_ShallowCopy(v interface{}) {
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamRequest_Dial) Proto_ShallowClone() (c *StreamRequest_Dial) {
	if x != nil {
		c = new(StreamRequest_Dial)
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamResponse) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *StreamResponse:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface{ GetData() isStreamResponse_Data }); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface {
					GetConnect() *StreamResponse_Connect
				}); ok {
					var defaultValue *StreamResponse_Connect
					if v := v.GetConnect(); v != defaultValue {
						x.Data = &StreamResponse_Connect_{Connect: v}
						return
					}
				}
				if v, ok := v.(interface{ GetDial() *StreamResponse_Dial }); ok {
					var defaultValue *StreamResponse_Dial
					if v := v.GetDial(); v != defaultValue {
						x.Data = &StreamResponse_Dial_{Dial: v}
						return
					}
				}
				if v, ok := v.(interface{ GetTunnel() *grpc.ClientToServer }); ok {
					var defaultValue *grpc.ClientToServer
					if v := v.GetTunnel(); v != defaultValue {
						x.Data = &StreamResponse_Tunnel{Tunnel: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamResponse) Proto_ShallowClone() (c *StreamResponse) {
	if x != nil {
		c = new(StreamResponse)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamResponse_Connect) Proto_ShallowCopy(v interface{}) {
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamResponse_Connect) Proto_ShallowClone() (c *StreamResponse_Connect) {
	if x != nil {
		c = new(StreamResponse_Connect)
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *StreamResponse_Dial) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *StreamResponse_Dial:
		x.Endpoint = v.GetEndpoint()
	default:
		if v, ok := v.(interface{ GetEndpoint() string }); ok {
			x.Endpoint = v.GetEndpoint()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *StreamResponse_Dial) Proto_ShallowClone() (c *StreamResponse_Dial) {
	if x != nil {
		c = new(StreamResponse_Dial)
		c.Endpoint = x.Endpoint
	}
	return
}
