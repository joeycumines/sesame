package netconn

import (
	"strings"
	"testing"
	"time"

	sesamepb "github.com/joeycumines/sesame/sesamepb"
	typepb "github.com/joeycumines/sesame/typepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestValidateDialRequest(t *testing.T) {
	// Every client-controlled dial field rejects malformed input with
	// INVALID_ARGUMENT at the boundary, before any dialing (AIP-193),
	// matching the TypeScript reference's netConn boundary checks.
	dial := func(addr string) *sesamepb.NetConnRequest_Dial {
		return &sesamepb.NetConnRequest_Dial{Address: &typepb.NetAddr{Network: "tcp", Address: addr}}
	}

	for _, tc := range []struct {
		name string
		req  *sesamepb.NetConnRequest_Dial
		want string // message fragment; empty means the request must validate
	}{
		{"valid host", dial("target.internal:443"), ""},
		{"valid ipv4", dial("127.0.0.1:1"), ""},
		{"valid bracketed ipv6", dial("[::1]:443"), ""},
		{"missing address", &sesamepb.NetConnRequest_Dial{}, "missing target address"},
		{"empty address", dial(""), "missing target address"},
		{"no port", dial("example.com"), "invalid target address"},
		{"bare ipv6 literal", dial("::1"), "invalid target address"},
		{"multiple separators", dial("host:80:90"), "invalid target address"},
		{"port zero", dial("host:0"), "invalid port"},
		{"port above range", dial("host:65536"), "invalid port"},
		{"signed port", dial("host:+80"), "invalid port"},
		{"empty port", dial("host:"), "invalid"},
		{"negative capability", func() *sesamepb.NetConnRequest_Dial {
			req := dial("target.internal:443")
			req.Capabilities = &sesamepb.NetConnRequest_Capabilities{InitialWindowSize: -1}
			return req
		}(), "negative capability value"},
		{"impossible tls range", func() *sesamepb.NetConnRequest_Dial {
			req := dial("target.internal:443")
			req.Tls = &sesamepb.TlsOptions{MinVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_3, MaxVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_2}
			return req
		}(), "min_version exceeds max_version"},
		{"negative timeout", func() *sesamepb.NetConnRequest_Dial {
			req := dial("target.internal:443")
			req.Timeout = durationpb.New(-time.Second)
			return req
		}(), "negative dial timeout"},
		{"zero timeout is unset", func() *sesamepb.NetConnRequest_Dial {
			req := dial("target.internal:443")
			req.Timeout = durationpb.New(0)
			return req
		}(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDialRequest(tc.req)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected the request to validate, got %v", err)
				}
				return
			}
			if err == nil || status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected InvalidArgument (%s), got %v", tc.want, err)
			}
		})
	}
}
