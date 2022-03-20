// Code generated by protoc-gen-go-copy. DO NOT EDIT.
// source: sesame/v1alpha1/sesame.proto

package api

import "github.com/joeycumines/sesame/genproto/type/grpcmetadata"
import "google.golang.org/genproto/googleapis/rpc/status"
import "google.golang.org/protobuf/types/known/anypb"
import "google.golang.org/protobuf/types/known/fieldmaskpb"
import "google.golang.org/protobuf/types/known/timestamppb"

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *Remote) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *Remote:
		x.Name = v.GetName()
		x.DisplayName = v.GetDisplayName()
		x.Description = v.GetDescription()
		x.CreateTime = v.GetCreateTime()
	default:
		if v, ok := v.(interface{ GetName() string }); ok {
			x.Name = v.GetName()
		}
		if v, ok := v.(interface{ GetDisplayName() string }); ok {
			x.DisplayName = v.GetDisplayName()
		}
		if v, ok := v.(interface{ GetDescription() string }); ok {
			x.Description = v.GetDescription()
		}
		if v, ok := v.(interface{ GetCreateTime() *timestamppb.Timestamp }); ok {
			x.CreateTime = v.GetCreateTime()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *Remote) Proto_ShallowClone() (c *Remote) {
	if x != nil {
		c = new(Remote)
		c.Name = x.Name
		c.DisplayName = x.DisplayName
		c.Description = x.Description
		c.CreateTime = x.CreateTime
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *Namespace) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *Namespace:
		x.Name = v.GetName()
	default:
		if v, ok := v.(interface{ GetName() string }); ok {
			x.Name = v.GetName()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *Namespace) Proto_ShallowClone() (c *Namespace) {
	if x != nil {
		c = new(Namespace)
		c.Name = x.Name
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *CreateRemoteRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *CreateRemoteRequest:
		x.Parent = v.GetParent()
		x.Remote = v.GetRemote()
	default:
		if v, ok := v.(interface{ GetParent() string }); ok {
			x.Parent = v.GetParent()
		}
		if v, ok := v.(interface{ GetRemote() *Remote }); ok {
			x.Remote = v.GetRemote()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *CreateRemoteRequest) Proto_ShallowClone() (c *CreateRemoteRequest) {
	if x != nil {
		c = new(CreateRemoteRequest)
		c.Parent = x.Parent
		c.Remote = x.Remote
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *GetRemoteRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *GetRemoteRequest:
		x.Name = v.GetName()
	default:
		if v, ok := v.(interface{ GetName() string }); ok {
			x.Name = v.GetName()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *GetRemoteRequest) Proto_ShallowClone() (c *GetRemoteRequest) {
	if x != nil {
		c = new(GetRemoteRequest)
		c.Name = x.Name
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *DeleteRemoteRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *DeleteRemoteRequest:
		x.Name = v.GetName()
	default:
		if v, ok := v.(interface{ GetName() string }); ok {
			x.Name = v.GetName()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *DeleteRemoteRequest) Proto_ShallowClone() (c *DeleteRemoteRequest) {
	if x != nil {
		c = new(DeleteRemoteRequest)
		c.Name = x.Name
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *UpdateRemoteRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *UpdateRemoteRequest:
		x.Remote = v.GetRemote()
		x.UpdateMask = v.GetUpdateMask()
	default:
		if v, ok := v.(interface{ GetRemote() *Remote }); ok {
			x.Remote = v.GetRemote()
		}
		if v, ok := v.(interface{ GetUpdateMask() *fieldmaskpb.FieldMask }); ok {
			x.UpdateMask = v.GetUpdateMask()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *UpdateRemoteRequest) Proto_ShallowClone() (c *UpdateRemoteRequest) {
	if x != nil {
		c = new(UpdateRemoteRequest)
		c.Remote = x.Remote
		c.UpdateMask = x.UpdateMask
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ListRemotesRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ListRemotesRequest:
		x.Parent = v.GetParent()
		x.PageSize = v.GetPageSize()
		x.PageToken = v.GetPageToken()
	default:
		if v, ok := v.(interface{ GetParent() string }); ok {
			x.Parent = v.GetParent()
		}
		if v, ok := v.(interface{ GetPageSize() rune }); ok {
			x.PageSize = v.GetPageSize()
		}
		if v, ok := v.(interface{ GetPageToken() string }); ok {
			x.PageToken = v.GetPageToken()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ListRemotesRequest) Proto_ShallowClone() (c *ListRemotesRequest) {
	if x != nil {
		c = new(ListRemotesRequest)
		c.Parent = x.Parent
		c.PageSize = x.PageSize
		c.PageToken = x.PageToken
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *ListRemotesResponse) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *ListRemotesResponse:
		x.Remotes = v.GetRemotes()
		x.NextPageToken = v.GetNextPageToken()
	default:
		if v, ok := v.(interface{ GetRemotes() []*Remote }); ok {
			x.Remotes = v.GetRemotes()
		}
		if v, ok := v.(interface{ GetNextPageToken() string }); ok {
			x.NextPageToken = v.GetNextPageToken()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *ListRemotesResponse) Proto_ShallowClone() (c *ListRemotesResponse) {
	if x != nil {
		c = new(ListRemotesResponse)
		c.Remotes = x.Remotes
		c.NextPageToken = x.NextPageToken
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *CallMethodRequest) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *CallMethodRequest:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface {
			GetData() isCallMethodRequest_Data
		}); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface {
					GetDial() *CallMethodRequest_Dial
				}); ok {
					var defaultValue *CallMethodRequest_Dial
					if v := v.GetDial(); v != defaultValue {
						x.Data = &CallMethodRequest_Dial_{Dial: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetHeader() *grpcmetadata.GrpcMetadata
				}); ok {
					var defaultValue *grpcmetadata.GrpcMetadata
					if v := v.GetHeader(); v != defaultValue {
						x.Data = &CallMethodRequest_Header{Header: v}
						return
					}
				}
				if v, ok := v.(interface{ GetContent() *anypb.Any }); ok {
					var defaultValue *anypb.Any
					if v := v.GetContent(); v != defaultValue {
						x.Data = &CallMethodRequest_Content{Content: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *CallMethodRequest) Proto_ShallowClone() (c *CallMethodRequest) {
	if x != nil {
		c = new(CallMethodRequest)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *CallMethodRequest_Dial) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *CallMethodRequest_Dial:
		x.Name = v.GetName()
	default:
		if v, ok := v.(interface{ GetName() string }); ok {
			x.Name = v.GetName()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *CallMethodRequest_Dial) Proto_ShallowClone() (c *CallMethodRequest_Dial) {
	if x != nil {
		c = new(CallMethodRequest_Dial)
		c.Name = x.Name
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *CallMethodResponse) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *CallMethodResponse:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface {
			GetData() isCallMethodResponse_Data
		}); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface {
					GetConn() *CallMethodResponse_Conn
				}); ok {
					var defaultValue *CallMethodResponse_Conn
					if v := v.GetConn(); v != defaultValue {
						x.Data = &CallMethodResponse_Conn_{Conn: v}
						return
					}
				}
				if v, ok := v.(interface{ GetError() *status.Status }); ok {
					var defaultValue *status.Status
					if v := v.GetError(); v != defaultValue {
						x.Data = &CallMethodResponse_Error{Error: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetHeader() *grpcmetadata.GrpcMetadata
				}); ok {
					var defaultValue *grpcmetadata.GrpcMetadata
					if v := v.GetHeader(); v != defaultValue {
						x.Data = &CallMethodResponse_Header{Header: v}
						return
					}
				}
				if v, ok := v.(interface{ GetContent() *anypb.Any }); ok {
					var defaultValue *anypb.Any
					if v := v.GetContent(); v != defaultValue {
						x.Data = &CallMethodResponse_Content{Content: v}
						return
					}
				}
				if v, ok := v.(interface {
					GetTrailer() *grpcmetadata.GrpcMetadata
				}); ok {
					var defaultValue *grpcmetadata.GrpcMetadata
					if v := v.GetTrailer(); v != defaultValue {
						x.Data = &CallMethodResponse_Trailer{Trailer: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *CallMethodResponse) Proto_ShallowClone() (c *CallMethodResponse) {
	if x != nil {
		c = new(CallMethodResponse)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *CallMethodResponse_Conn) Proto_ShallowCopy(v interface{}) {
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *CallMethodResponse_Conn) Proto_ShallowClone() (c *CallMethodResponse_Conn) {
	if x != nil {
		c = new(CallMethodResponse_Conn)
	}
	return
}
