// Code generated by protoc-gen-go-copy. DO NOT EDIT.
// source: sesame/type/grpcmetadata.proto

package grpcmetadata

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *GrpcMetadata) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *GrpcMetadata:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface {
			GetData() map[string]*GrpcMetadata_Value
		}); ok {
			x.Data = v.GetData()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *GrpcMetadata) Proto_ShallowClone() (c *GrpcMetadata) {
	if x != nil {
		c = new(GrpcMetadata)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *GrpcMetadata_Value) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *GrpcMetadata_Value:
		x.Data = v.GetData()
	default:
		if v, ok := v.(interface {
			GetData() isGrpcMetadata_Value_Data
		}); ok {
			x.Data = v.GetData()
		} else {
			func() {
				if v, ok := v.(interface{ GetStr() *GrpcMetadata_Str }); ok {
					var defaultValue *GrpcMetadata_Str
					if v := v.GetStr(); v != defaultValue {
						x.Data = &GrpcMetadata_Value_Str{Str: v}
						return
					}
				}
				if v, ok := v.(interface{ GetBin() *GrpcMetadata_Bin }); ok {
					var defaultValue *GrpcMetadata_Bin
					if v := v.GetBin(); v != defaultValue {
						x.Data = &GrpcMetadata_Value_Bin{Bin: v}
						return
					}
				}
			}()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *GrpcMetadata_Value) Proto_ShallowClone() (c *GrpcMetadata_Value) {
	if x != nil {
		c = new(GrpcMetadata_Value)
		c.Data = x.Data
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *GrpcMetadata_Str) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *GrpcMetadata_Str:
		x.Values = v.GetValues()
	default:
		if v, ok := v.(interface{ GetValues() []string }); ok {
			x.Values = v.GetValues()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *GrpcMetadata_Str) Proto_ShallowClone() (c *GrpcMetadata_Str) {
	if x != nil {
		c = new(GrpcMetadata_Str)
		c.Values = x.Values
	}
	return
}

// Proto_ShallowCopy copies fields, from v to the receiver, using field getters.
// Note that v is of an arbitrary type, which may implement any number of the
// field getters, which are defined as any methods of the same signature as those
// generated for the receiver type, with a name starting with Get.
func (x *GrpcMetadata_Bin) Proto_ShallowCopy(v interface{}) {
	switch v := v.(type) {
	case *GrpcMetadata_Bin:
		x.Values = v.GetValues()
	default:
		if v, ok := v.(interface{ GetValues() [][]byte }); ok {
			x.Values = v.GetValues()
		}
	}
}

// Proto_ShallowClone returns a shallow copy of the receiver or nil if it's nil.
func (x *GrpcMetadata_Bin) Proto_ShallowClone() (c *GrpcMetadata_Bin) {
	if x != nil {
		c = new(GrpcMetadata_Bin)
		c.Values = x.Values
	}
	return
}