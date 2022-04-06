// Code generated by protoc-gen-go-copy. DO NOT EDIT.
// source: sesame/type/grpctunnel.proto

package grpctunnel

import "github.com/joeycumines/sesame/genproto/type/grpcmetadata"
import "google.golang.org/genproto/googleapis/rpc/status"
import "google.golang.org/protobuf/types/known/emptypb"

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ClientToServer) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ClientToServer:
		x.StreamId = v.GetStreamId()
		x.Frame = v.GetFrame()
	default:
		if v, ok := v.(interface{ GetStreamId() uint64 }); ok {
			x.StreamId = v.GetStreamId()
		}
		if v, ok := v.(interface{ GetFrame() isClientToServer_Frame }); ok {
			x.Frame = v.GetFrame()
		} else {
			func() {
				if v, ok := v.(interface{ GetWindowUpdate() uint32 }); ok {
					var defaultValue uint32
					if v := v.GetWindowUpdate(); v != defaultValue {
						x.Frame = &ClientToServer_WindowUpdate{WindowUpdate: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetNewStream() *ClientToServer_NewStream
				}); ok {
					var defaultValue *ClientToServer_NewStream
					if v := v.GetNewStream(); v != defaultValue {
						x.Frame = &ClientToServer_NewStream_{NewStream: v}
						return
					}
				}
				if v, ok := v.(interface{ GetMessage() *EncodedMessage }); ok {
					var defaultValue *EncodedMessage
					if v := v.GetMessage(); v != defaultValue {
						x.Frame = &ClientToServer_Message{Message: v}
						return
					}
				}
				if v, ok := v.(interface{ GetMessageData() []byte }); ok {
					if v := v.GetMessageData(); v != nil {
						x.Frame = &ClientToServer_MessageData{MessageData: v}
						return
					}
				}
				if v, ok := v.(interface{ GetHalfClose() *emptypb.Empty }); ok {
					var defaultValue *emptypb.Empty
					if v := v.GetHalfClose(); v != defaultValue {
						x.Frame = &ClientToServer_HalfClose{HalfClose: v}
						return
					}
				}
				if v, ok := v.(interface{ GetCancel() *emptypb.Empty }); ok {
					var defaultValue *emptypb.Empty
					if v := v.GetCancel(); v != defaultValue {
						x.Frame = &ClientToServer_Cancel{Cancel: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ClientToServer) Proto_ShallowClone() (c *ClientToServer) {
	if x != nil {
		c = new(ClientToServer)
		c.StreamId = x.StreamId
		c.Frame = x.Frame
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ClientToServer_NewStream) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ClientToServer_NewStream:
		x.Method = v.GetMethod()
		x.Header = v.GetHeader()
	default:
		if v, ok := v.(interface{ GetMethod() string }); ok {
			x.Method = v.GetMethod()
		}
		if v, ok := v.(interface {
			GetHeader() *grpcmetadata.GrpcMetadata
		}); ok {
			x.Header = v.GetHeader()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ClientToServer_NewStream) Proto_ShallowClone() (c *ClientToServer_NewStream) {
	if x != nil {
		c = new(ClientToServer_NewStream)
		c.Method = x.Method
		c.Header = x.Header
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ServerToClient) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ServerToClient:
		x.StreamId = v.GetStreamId()
		x.Frame = v.GetFrame()
	default:
		if v, ok := v.(interface{ GetStreamId() uint64 }); ok {
			x.StreamId = v.GetStreamId()
		}
		if v, ok := v.(interface{ GetFrame() isServerToClient_Frame }); ok {
			x.Frame = v.GetFrame()
		} else {
			func() {
				if v, ok := v.(interface{ GetWindowUpdate() uint32 }); ok {
					var defaultValue uint32
					if v := v.GetWindowUpdate(); v != defaultValue {
						x.Frame = &ServerToClient_WindowUpdate{WindowUpdate: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetHeader() *grpcmetadata.GrpcMetadata
				}); ok {
					var defaultValue *grpcmetadata.GrpcMetadata
					if v := v.GetHeader(); v != defaultValue {
						x.Frame = &ServerToClient_Header{Header: v}
						return
					}
				}
				if v, ok := v.(interface{ GetMessage() *EncodedMessage }); ok {
					var defaultValue *EncodedMessage
					if v := v.GetMessage(); v != defaultValue {
						x.Frame = &ServerToClient_Message{Message: v}
						return
					}
				}
				if v, ok := v.(interface{ GetMessageData() []byte }); ok {
					if v := v.GetMessageData(); v != nil {
						x.Frame = &ServerToClient_MessageData{MessageData: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetCloseStream() *ServerToClient_CloseStream
				}); ok {
					var defaultValue *ServerToClient_CloseStream
					if v := v.GetCloseStream(); v != defaultValue {
						x.Frame = &ServerToClient_CloseStream_{CloseStream: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ServerToClient) Proto_ShallowClone() (c *ServerToClient) {
	if x != nil {
		c = new(ServerToClient)
		c.StreamId = x.StreamId
		c.Frame = x.Frame
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ServerToClient_CloseStream) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ServerToClient_CloseStream:
		x.Trailer = v.GetTrailer()
		x.Status = v.GetStatus()
	default:
		if v, ok := v.(interface {
			GetTrailer() *grpcmetadata.GrpcMetadata
		}); ok {
			x.Trailer = v.GetTrailer()
		}
		if v, ok := v.(interface{ GetStatus() *status.Status }); ok {
			x.Status = v.GetStatus()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ServerToClient_CloseStream) Proto_ShallowClone() (c *ServerToClient_CloseStream) {
	if x != nil {
		c = new(ServerToClient_CloseStream)
		c.Trailer = x.Trailer
		c.Status = x.Status
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *EncodedMessage) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *EncodedMessage:
		x.Size = v.GetSize()
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface{ GetSize() rune }); ok {
			x.Size = v.GetSize()
		}
		if v, ok := v.(interface{ GetData() []byte }); ok {
			x.Data = v.GetData()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *EncodedMessage) Proto_ShallowClone() (c *EncodedMessage) {
	if x != nil {
		c = new(EncodedMessage)
		c.Size = x.Size
		c.Data = x.Data
	}
	return
}