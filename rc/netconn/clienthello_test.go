package netconn

import (
	cryptotls "crypto/tls"
	"testing"

	sesametls "github.com/joeycumines/sesame/rc/tls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGoClientHelloCapabilities(t *testing.T) {
	caps := GoClientHelloCapabilities()
	if caps.CustomSignatureAlgorithms || caps.CustomExtensionOrder || caps.RawExtensions ||
		caps.GreaseValues || caps.SessionIdLength || caps.PaddingControl ||
		caps.LegacyVersionControl || caps.CompressionMethods {
		t.Fatal("Go crypto/tls caps must not over-advertise unsupported dimensions")
	}
	if !caps.CustomCipherSuites || !caps.CustomSupportedGroups {
		t.Fatal("Go crypto/tls caps must advertise cipher suite and group control")
	}
}

func TestApplyGoClientHelloSpec_FailsClosedOnUnhonorableDimensions(t *testing.T) {
	const (
		maxTLS11 = uint16(cryptotls.VersionTLS11)
		maxTLS12 = uint16(cryptotls.VersionTLS12)
		maxTLS13 = uint16(cryptotls.VersionTLS13)
	)

	cases := []struct {
		name         string
		spec         *sesametls.ClientHelloSpec
		effectiveMax uint16
	}{
		{"signature algorithms have no public API",
			&sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{0x0403}}, maxTLS13},
		{"extension list has no public API",
			&sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{{Type: 43}}}, maxTLS13},
		{"compression other than null",
			&sesametls.ClientHelloSpec{CompressionMethods: []int32{1}}, maxTLS13},
		{"multi-method compression",
			&sesametls.ClientHelloSpec{CompressionMethods: []int32{0, 1}}, maxTLS13},
		{"session id omit sentinel",
			&sesametls.ClientHelloSpec{SessionIdLength: -1}, maxTLS13},
		{"session id short length",
			&sesametls.ClientHelloSpec{SessionIdLength: 16}, maxTLS13},
		{"legacy version pinned",
			&sesametls.ClientHelloSpec{LegacyVersion: sesametls.TLSVersion_TLS_1_2}, maxTLS13},
		{"padding control",
			&sesametls.ClientHelloSpec{PadToSize: 512}, maxTLS13},
		{"cipher suites with TLS1.3 max append fixed suites",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0xc02b}}, maxTLS13},
		{"cipher suites with sub-TLS1.2 max",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0xc02b}}, maxTLS11},
		{"cipher order reversed against engine preference",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0x0035, 0x002f}}, maxTLS12},
		{"tls13-only cipher at tls12 max",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0x1301}}, maxTLS12},
		{"group order reversed against engine preference",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{23, 29}}, maxTLS13},
		{"unknown group silently dropped by engine",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{0x7a7a}}, maxTLS13},
		{"post-quantum group below tls13",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{4588, 29}}, maxTLS12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &cryptotls.Config{}
			err := ApplyGoClientHelloSpec(tc.spec, cfg, tc.effectiveMax)
			if err == nil {
				t.Fatal("expected FailedPrecondition, got nil")
			}
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("expected FailedPrecondition, got %v", err)
			}
		})
	}
}

func TestApplyGoClientHelloSpec_AppliesHonorableDimensions(t *testing.T) {
	// [0x002f, 0x0035] and [29, 23] are in Go's fixed preference order in
	// BOTH the AES-hardware and no-AES-hardware tables, so these assertions
	// are hardware-independent.
	cfg := &cryptotls.Config{}
	spec := &sesametls.ClientHelloSpec{
		CipherSuites:       []int32{0x002f, 0x0035},
		SupportedGroups:    []int32{29, 23},
		CompressionMethods: []int32{0},
		SessionIdLength:    32,
	}
	if err := ApplyGoClientHelloSpec(spec, cfg, uint16(cryptotls.VersionTLS12)); err != nil {
		t.Fatalf("expected spec to be honorable, got %v", err)
	}
	if len(cfg.CipherSuites) != 2 || cfg.CipherSuites[0] != 0x002f || cfg.CipherSuites[1] != 0x0035 {
		t.Fatalf("unexpected CipherSuites: %v", cfg.CipherSuites)
	}
	if len(cfg.CurvePreferences) != 2 || cfg.CurvePreferences[0] != 29 || cfg.CurvePreferences[1] != 23 {
		t.Fatalf("unexpected CurvePreferences: %v", cfg.CurvePreferences)
	}
}

func TestApplyGoClientHelloSpec_EmptySpecIsEngineDefault(t *testing.T) {
	cfg := &cryptotls.Config{}
	if err := ApplyGoClientHelloSpec(&sesametls.ClientHelloSpec{}, cfg, uint16(cryptotls.VersionTLS13)); err != nil {
		t.Fatalf("empty spec must select engine defaults, got %v", err)
	}
	if cfg.CipherSuites != nil || cfg.CurvePreferences != nil {
		t.Fatal("empty spec must not alter engine defaults")
	}
	if err := ApplyGoClientHelloSpec(nil, cfg, uint16(cryptotls.VersionTLS13)); err != nil {
		t.Fatalf("nil spec must select engine defaults, got %v", err)
	}
}

func TestVerifyAppliedClientHello(t *testing.T) {
	spec := &sesametls.ClientHelloSpec{
		CipherSuites:    []int32{0xc02f},
		SupportedGroups: []int32{29},
		SessionIdLength: 32,
		Extensions: []*sesametls.ClientHelloExtension{
			{Type: 0, Body: &sesametls.ClientHelloExtension_Raw{Raw: []byte{1, 2, 3}}},
			{Type: 16, Body: &sesametls.ClientHelloExtension_Auto{Auto: &sesametls.AutoExtensionBody{}}},
		},
	}

	if err := VerifyAppliedClientHello(nil, nil); err != nil {
		t.Fatalf("nil/nil must verify: %v", err)
	}
	if err := VerifyAppliedClientHello(spec, nil); err == nil {
		t.Fatal("missing echo must fail closed")
	}
	if err := VerifyAppliedClientHello(nil, spec); err == nil {
		t.Fatal("unrequested echo must fail closed")
	}
	if err := VerifyAppliedClientHello(spec, proto.Clone(spec).(*sesametls.ClientHelloSpec)); err != nil {
		t.Fatalf("identical echo must verify: %v", err)
	}

	mutated := proto.Clone(spec).(*sesametls.ClientHelloSpec)
	mutated.SessionIdLength = 0
	if err := VerifyAppliedClientHello(spec, mutated); err == nil {
		t.Fatal("mutated scalar echo must fail closed")
	}

	mutatedRaw := proto.Clone(spec).(*sesametls.ClientHelloSpec)
	mutatedRaw.Extensions[0].Body = &sesametls.ClientHelloExtension_Raw{Raw: []byte{1, 2, 4}}
	if err := VerifyAppliedClientHello(spec, mutatedRaw); err == nil {
		t.Fatal("mutated raw extension body must fail closed")
	}
}
