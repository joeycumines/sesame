package typepb

// Hand-written conversions between the forward and reverse tunnel frame
// vocabularies. The two vocabularies are wire-identical in shape (see
// schema/sesame/type/grpctunnel.proto): OpenTunnelRequest matches
// OpenReverseTunnelResponse (client-shaped frames) and OpenTunnelResponse
// matches OpenReverseTunnelRequest (server-shaped frames). The generated
// Proto_ShallowCopy cannot be used for these cross-type oneof copies: its
// generated fallback probes assert on the zero value of the failed
// interface assertion (Go short-variable-declaration shadowing), so the
// frame oneof would be silently dropped.

func TunnelRequestFromReverseResponse(msg *OpenReverseTunnelResponse) *OpenTunnelRequest {
	if msg == nil {
		return nil
	}
	out := &OpenTunnelRequest{StreamId: msg.GetStreamId()}
	switch frame := msg.GetFrame().(type) {
	case *OpenReverseTunnelResponse_WindowUpdate:
		out.Frame = &OpenTunnelRequest_WindowUpdate{WindowUpdate: frame.WindowUpdate}
	case *OpenReverseTunnelResponse_NewStream:
		out.Frame = &OpenTunnelRequest_NewStream{NewStream: frame.NewStream}
	case *OpenReverseTunnelResponse_Message:
		out.Frame = &OpenTunnelRequest_Message{Message: frame.Message}
	case *OpenReverseTunnelResponse_MessageData:
		out.Frame = &OpenTunnelRequest_MessageData{MessageData: frame.MessageData}
	case *OpenReverseTunnelResponse_HalfClose:
		out.Frame = &OpenTunnelRequest_HalfClose{HalfClose: frame.HalfClose}
	case *OpenReverseTunnelResponse_Cancel:
		out.Frame = &OpenTunnelRequest_Cancel{Cancel: frame.Cancel}
	}
	return out
}

func ReverseResponseFromTunnelRequest(msg *OpenTunnelRequest) *OpenReverseTunnelResponse {
	if msg == nil {
		return nil
	}
	out := &OpenReverseTunnelResponse{StreamId: msg.GetStreamId()}
	switch frame := msg.GetFrame().(type) {
	case *OpenTunnelRequest_WindowUpdate:
		out.Frame = &OpenReverseTunnelResponse_WindowUpdate{WindowUpdate: frame.WindowUpdate}
	case *OpenTunnelRequest_NewStream:
		out.Frame = &OpenReverseTunnelResponse_NewStream{NewStream: frame.NewStream}
	case *OpenTunnelRequest_Message:
		out.Frame = &OpenReverseTunnelResponse_Message{Message: frame.Message}
	case *OpenTunnelRequest_MessageData:
		out.Frame = &OpenReverseTunnelResponse_MessageData{MessageData: frame.MessageData}
	case *OpenTunnelRequest_HalfClose:
		out.Frame = &OpenReverseTunnelResponse_HalfClose{HalfClose: frame.HalfClose}
	case *OpenTunnelRequest_Cancel:
		out.Frame = &OpenReverseTunnelResponse_Cancel{Cancel: frame.Cancel}
	}
	return out
}

func TunnelResponseFromReverseRequest(msg *OpenReverseTunnelRequest) *OpenTunnelResponse {
	if msg == nil {
		return nil
	}
	out := &OpenTunnelResponse{StreamId: msg.GetStreamId()}
	switch frame := msg.GetFrame().(type) {
	case *OpenReverseTunnelRequest_WindowUpdate:
		out.Frame = &OpenTunnelResponse_WindowUpdate{WindowUpdate: frame.WindowUpdate}
	case *OpenReverseTunnelRequest_Header:
		out.Frame = &OpenTunnelResponse_Header{Header: frame.Header}
	case *OpenReverseTunnelRequest_Message:
		out.Frame = &OpenTunnelResponse_Message{Message: frame.Message}
	case *OpenReverseTunnelRequest_MessageData:
		out.Frame = &OpenTunnelResponse_MessageData{MessageData: frame.MessageData}
	case *OpenReverseTunnelRequest_CloseStream:
		out.Frame = &OpenTunnelResponse_CloseStream{CloseStream: frame.CloseStream}
	}
	return out
}

func ReverseRequestFromTunnelResponse(msg *OpenTunnelResponse) *OpenReverseTunnelRequest {
	if msg == nil {
		return nil
	}
	out := &OpenReverseTunnelRequest{StreamId: msg.GetStreamId()}
	switch frame := msg.GetFrame().(type) {
	case *OpenTunnelResponse_WindowUpdate:
		out.Frame = &OpenReverseTunnelRequest_WindowUpdate{WindowUpdate: frame.WindowUpdate}
	case *OpenTunnelResponse_Header:
		out.Frame = &OpenReverseTunnelRequest_Header{Header: frame.Header}
	case *OpenTunnelResponse_Message:
		out.Frame = &OpenReverseTunnelRequest_Message{Message: frame.Message}
	case *OpenTunnelResponse_MessageData:
		out.Frame = &OpenReverseTunnelRequest_MessageData{MessageData: frame.MessageData}
	case *OpenTunnelResponse_CloseStream:
		out.Frame = &OpenReverseTunnelRequest_CloseStream{CloseStream: frame.CloseStream}
	}
	return out
}
