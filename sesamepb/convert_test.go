package sesamepb

import (
	"testing"

	typepb "github.com/joeycumines/sesame/typepb"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestTunnelFrameVocabularyConversions pins that the hand-written conversions
// between the forward and reverse tunnel frame vocabularies preserve the
// serialized bytes of every frame member.
func TestTunnelFrameVocabularyConversions(t *testing.T) {
	clientShaped := []*OpenTunnelRequest{
		{StreamId: 7, Frame: &OpenTunnelRequest_NewStream{NewStream: &NewStream{Method: "a/b/c", Header: &typepb.GrpcMetadata{}}}},
		{StreamId: 8, Frame: &OpenTunnelRequest_Message{Message: &EncodedMessage{Size: 3, Data: []byte("abc")}}},
		{StreamId: 9, Frame: &OpenTunnelRequest_MessageData{MessageData: []byte("xyz")}},
		{StreamId: 10, Frame: &OpenTunnelRequest_HalfClose{HalfClose: &emptypb.Empty{}}},
		{StreamId: 11, Frame: &OpenTunnelRequest_Cancel{Cancel: &emptypb.Empty{}}},
		{StreamId: 12, Frame: &OpenTunnelRequest_WindowUpdate{WindowUpdate: 42}},
		{StreamId: 13},
	}
	serverShaped := []*OpenTunnelResponse{
		{StreamId: 14, Frame: &OpenTunnelResponse_Header{Header: &typepb.GrpcMetadata{}}},
		{StreamId: 15, Frame: &OpenTunnelResponse_Message{Message: &EncodedMessage{Size: 2, Data: []byte("hi")}}},
		{StreamId: 16, Frame: &OpenTunnelResponse_MessageData{MessageData: []byte("mn")}},
		{StreamId: 17, Frame: &OpenTunnelResponse_CloseStream{CloseStream: &CloseStream{}}},
		{StreamId: 18, Frame: &OpenTunnelResponse_WindowUpdate{WindowUpdate: 7}},
		{StreamId: 19},
	}

	for _, m := range clientShaped {
		want, err := proto.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		got, err := proto.Marshal(TunnelRequestFromReverseResponse(ReverseResponseFromTunnelRequest(m)))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("client-shaped frame %d: round trip changed bytes: %q -> %q", m.GetStreamId(), want, got)
		}
	}
	for _, m := range serverShaped {
		want, err := proto.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		got, err := proto.Marshal(TunnelResponseFromReverseRequest(ReverseRequestFromTunnelResponse(m)))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("server-shaped frame %d: round trip changed bytes: %q -> %q", m.GetStreamId(), want, got)
		}
	}
}
