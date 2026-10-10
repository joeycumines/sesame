package netconn

import (
	cryptotls "crypto/tls"
	"math"
	"runtime"
	"slices"

	sesametls "github.com/joeycumines/sesame/rc/tls"
	"golang.org/x/sys/cpu"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Go crypto/tls ClientHello engine.
//
// Empirical ground truth (wire-captured, Go 1.27): Config.CipherSuites and
// Config.CurvePreferences are MEMBERSHIP FILTERS over fixed internal
// preference orders. User-specified order is never honored on the wire.
// Unknown curve IDs are silently dropped. TLS 1.3 cipher suites are always
// appended in fixed order when max >= TLS1.3. session_id is always 32 bytes.
// There is no public control for signature algorithms, extension order,
// padding, compression, or legacy version.
//
// The engine therefore accepts a ClientHelloSpec only when every requested
// dimension exactly matches what crypto/tls will emit, and fails closed
// (FAILED_PRECONDITION) otherwise. Range violations are INVALID_ARGUMENT.

// goCipherSuitesPreferenceOrder mirrors crypto/tls.cipherSuitesPreferenceOrder
// (AES-hardware variant). Values are IANA identifiers in Go's fixed emission
// order. Keep in sync with the Go standard library.
var goCipherSuitesPreferenceOrder = []uint16{
	0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	0xc009, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	0xc013, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
	0xc00a, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	0xc014, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
	0x009c, // TLS_RSA_WITH_AES_128_GCM_SHA256
	0x009d, // TLS_RSA_WITH_AES_256_GCM_SHA384
	0x002f, // TLS_RSA_WITH_AES_128_CBC_SHA
	0x0035, // TLS_RSA_WITH_AES_256_CBC_SHA
	0xc012, // TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA
	0x000a, // TLS_RSA_WITH_3DES_EDE_CBC_SHA
	0xc023, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
	0xc027, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
	0x003c, // TLS_RSA_WITH_AES_128_CBC_SHA256
	0xc007, // TLS_ECDHE_ECDSA_WITH_RC4_128_SHA
	0xc011, // TLS_ECDHE_RSA_WITH_RC4_128_SHA
	0x0005, // TLS_RSA_WITH_RC4_128_SHA
}

// goCipherSuitesPreferenceOrderNoAES mirrors
// crypto/tls.cipherSuitesPreferenceOrderNoAES (ChaCha-preferred variant).
var goCipherSuitesPreferenceOrderNoAES = []uint16{
	0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	0xc009, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	0xc013, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
	0xc00a, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	0xc014, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
	0x009c, // TLS_RSA_WITH_AES_128_GCM_SHA256
	0x009d, // TLS_RSA_WITH_AES_256_GCM_SHA384
	0x002f, // TLS_RSA_WITH_AES_128_CBC_SHA
	0x0035, // TLS_RSA_WITH_AES_256_CBC_SHA
	0xc012, // TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA
	0x000a, // TLS_RSA_WITH_3DES_EDE_CBC_SHA
	0xc023, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
	0xc027, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
	0x003c, // TLS_RSA_WITH_AES_128_CBC_SHA256
	0xc007, // TLS_ECDHE_ECDSA_WITH_RC4_128_SHA
	0xc011, // TLS_ECDHE_RSA_WITH_RC4_128_SHA
	0x0005, // TLS_RSA_WITH_RC4_128_SHA
}

// goCurvePreferenceOrder mirrors crypto/tls.curvePreferenceOrder.
var goCurvePreferenceOrder = []uint16{
	4588, // X25519MLKEM768
	4587, // SecP256r1MLKEM768
	4589, // SecP384r1MLKEM1024
	514,  // MLKEM1024
	29,   // X25519
	23,   // CurveP256
	24,   // CurveP384
	25,   // CurveP521
}

// goTLS13OnlyCurves are key exchanges filtered out when version < TLS1.3.
var goTLS13OnlyCurves = map[uint16]bool{
	4588: true, // X25519MLKEM768
	4587: true, // SecP256r1MLKEM768
	4589: true, // SecP384r1MLKEM1024
	514:  true, // MLKEM1024
}

// goHasAESGCMHardware replicates crypto/tls's runtime AES-GCM hardware
// detection, which selects the cipher preference table.
func goHasAESGCMHardware() bool {
	switch runtime.GOARCH {
	case "amd64", "386":
		return cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ && cpu.X86.HasSSE41 && cpu.X86.HasSSSE3
	case "arm64":
		return cpu.ARM64.HasAES && cpu.ARM64.HasPMULL
	case "s390x":
		return cpu.S390X.HasAES && cpu.S390X.HasAESCTR && cpu.S390X.HasGHASH
	case "ppc64", "ppc64le":
		return true
	default:
		return false
	}
}

// GoClientHelloCapabilities returns the honest capability advertisement for
// the Go crypto/tls engine.
func GoClientHelloCapabilities() *sesametls.ClientHelloCapabilities {
	return &sesametls.ClientHelloCapabilities{
		CustomCipherSuites:        true,  // exact-match only, TLS<=1.2 effective max
		CustomSupportedGroups:     true,  // exact-match only
		CustomSignatureAlgorithms: false, // no public API
		CustomExtensionOrder:      false, // no public API
		RawExtensions:             false, // no public API
		GreaseValues:              false, // no public API
		SessionIdLength:           false, // always 32; only 0/32 accepted as compatible
		PaddingControl:            false, // no public API
		LegacyVersionControl:      false, // always TLS1.2 legacy
		CompressionMethods:        false, // always [0]
	}
}

// ValidateClientHelloSpec checks numeric ranges in the spec. Returns
// INVALID_ARGUMENT on any violation. Call before any engine-specific logic.
func ValidateClientHelloSpec(spec *sesametls.ClientHelloSpec) error {
	if spec == nil {
		return nil
	}
	for _, v := range spec.GetCipherSuites() {
		if v < 0 || v > math.MaxUint16 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: cipher_suites value out of range [0, 65535]: %d", v)
		}
	}
	for _, v := range spec.GetSupportedGroups() {
		if v < 0 || v > math.MaxUint16 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: supported_groups value out of range [0, 65535]: %d", v)
		}
	}
	for _, v := range spec.GetSignatureAlgorithms() {
		if v < 0 || v > math.MaxUint16 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: signature_algorithms value out of range [0, 65535]: %d", v)
		}
	}
	for _, ext := range spec.GetExtensions() {
		if ext == nil {
			return status.Error(codes.InvalidArgument, "sesame/rc/netconn: nil extension entry in client_hello.extensions")
		}
		if ext.GetType() < 0 || ext.GetType() > math.MaxUint16 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: extension type out of range [0, 65535]: %d", ext.GetType())
		}
	}
	for _, v := range spec.GetCompressionMethods() {
		if v < 0 || v > math.MaxUint8 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: compression_methods value out of range [0, 255]: %d", v)
		}
	}
	if sid := spec.GetSessionIdLength(); sid < -1 || sid > 32 {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: session_id_length out of range [-1, 32]: %d", sid)
	}
	if spec.GetPadToSize() < 0 {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: pad_to_size must not be negative: %d", spec.GetPadToSize())
	}
	return nil
}

// ApplyGoClientHelloSpec validates that the Go crypto/tls engine can honor
// the spec exactly, and applies the honorable dimensions to cfg. Returns
// FAILED_PRECONDITION if any dimension cannot be honored exactly.
//
// effectiveMaxVersion is the resolved max TLS version (from opts or Go
// default TLS1.3). It determines whether TLS 1.3 cipher suites would be
// appended (making exact cipher match impossible).
func ApplyGoClientHelloSpec(spec *sesametls.ClientHelloSpec, cfg *cryptotls.Config, effectiveMaxVersion uint16) error {
	if spec == nil {
		return nil
	}

	// --- cipher_suites ---
	if cs := spec.GetCipherSuites(); len(cs) > 0 {
		if effectiveMaxVersion >= cryptotls.VersionTLS13 {
			return status.Error(codes.FailedPrecondition,
				"sesame/rc/netconn: Go crypto/tls appends TLS 1.3 cipher suites in fixed order when max >= TLS1.3; cannot honor custom cipher_suites exactly")
		}
		if effectiveMaxVersion < cryptotls.VersionTLS12 {
			return status.Error(codes.FailedPrecondition,
				"sesame/rc/netconn: Go crypto/tls removes TLS 1.2-only suites when max < TLS1.2; cannot honor custom cipher_suites exactly")
		}
		// effectiveMax == TLS1.2: Go emits the preference table filtered by
		// membership. Verify the request matches that filtered output exactly.
		table := goCipherSuitesPreferenceOrder
		if !goHasAESGCMHardware() {
			table = goCipherSuitesPreferenceOrderNoAES
		}
		expected := filterByMembership(table, cs)
		if !uint16SliceEqual(expected, cs) {
			return status.Errorf(codes.FailedPrecondition,
				"sesame/rc/netconn: Go crypto/tls treats cipher_suites as a membership filter over its fixed preference order; requested order or content does not match the engine's emission (expected %v)", expected)
		}
		// Apply: set cfg.CipherSuites so Go uses membership-filter path.
		cfg.CipherSuites = make([]uint16, len(cs))
		for i, v := range cs {
			cfg.CipherSuites[i] = uint16(v)
		}
	}

	// --- supported_groups ---
	if sg := spec.GetSupportedGroups(); len(sg) > 0 {
		// Go emits curvePreferenceOrder filtered by membership in
		// CurvePreferences AND version constraints. Verify exact match.
		expected := filterGoCurves(sg, effectiveMaxVersion)
		if !uint16SliceEqual(expected, sg) {
			return status.Errorf(codes.FailedPrecondition,
				"sesame/rc/netconn: Go crypto/tls treats supported_groups as a membership filter over its fixed curve preference order; requested order or content does not match the engine's emission (expected %v)", expected)
		}
		// Apply: set CurvePreferences.
		cfg.CurvePreferences = make([]cryptotls.CurveID, len(sg))
		for i, v := range sg {
			cfg.CurvePreferences[i] = cryptotls.CurveID(v)
		}
	}

	// --- signature_algorithms ---
	if len(spec.GetSignatureAlgorithms()) > 0 {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: Go crypto/tls provides no public API for custom signature_algorithms; cannot honor this dimension")
	}

	// --- extensions ---
	if len(spec.GetExtensions()) > 0 {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: Go crypto/tls provides no public API for custom extension order or raw extensions; cannot honor this dimension")
	}

	// --- compression_methods ---
	if cm := spec.GetCompressionMethods(); len(cm) > 0 {
		// Go always sends [0] (null). Accept only [0] as compatible.
		if len(cm) != 1 || cm[0] != 0 {
			return status.Error(codes.FailedPrecondition,
				"sesame/rc/netconn: Go crypto/tls always sends compression_methods [0]; cannot honor custom compression methods")
		}
	}

	// --- session_id_length ---
	// Go always sends 32 bytes. Sentinel: 0=default(32), 32=explicit(32).
	// -1=omit and 1..31 are not possible.
	if sid := spec.GetSessionIdLength(); sid != 0 && sid != 32 {
		return status.Errorf(codes.FailedPrecondition,
			"sesame/rc/netconn: Go crypto/tls always sends a 32-byte session_id; cannot honor session_id_length=%d (only 0 or 32 are compatible)", sid)
	}

	// --- legacy_version ---
	if spec.GetLegacyVersion() != sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: Go crypto/tls always uses TLS 1.2 as the ClientHello legacy_version; cannot honor custom legacy_version")
	}

	// --- pad_to_size ---
	if spec.GetPadToSize() != 0 {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: Go crypto/tls provides no ClientHello padding control; cannot honor pad_to_size")
	}

	return nil
}

// filterByMembership returns the elements of table that appear in members,
// preserving table order. This replicates Go's Config.cipherSuites behavior
// when Config.CipherSuites is set.
func filterByMembership(table []uint16, members []int32) []uint16 {
	memberSet := make(map[uint16]bool, len(members))
	for _, m := range members {
		memberSet[uint16(m)] = true
	}
	var out []uint16
	for _, id := range table {
		if memberSet[id] {
			out = append(out, id)
		}
	}
	return out
}

// filterGoCurves returns the curves Go will emit given the requested groups
// and effective max version, replicating crypto/tls.curvePreferences +
// supportsCurve filtering.
func filterGoCurves(requested []int32, effectiveMaxVersion uint16) []uint16 {
	reqSet := make(map[uint16]bool, len(requested))
	for _, g := range requested {
		reqSet[uint16(g)] = true
	}
	var out []uint16
	for _, id := range goCurvePreferenceOrder {
		if !reqSet[id] {
			continue
		}
		// Version constraint: TLS1.3-only curves are filtered when < TLS1.3.
		if effectiveMaxVersion < cryptotls.VersionTLS13 && goTLS13OnlyCurves[id] {
			continue
		}
		out = append(out, id)
	}
	return out
}

// uint16SliceEqual compares a []uint16 against a []int32 element-wise.
func uint16SliceEqual(a []uint16, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != uint16(b[i]) {
			return false
		}
	}
	return true
}

// VerifyAppliedClientHello diffs the requested ClientHelloSpec against the
// server's applied_client_hello echo. Returns an error on mismatch (fail
// closed). Both nil is valid (no spec requested).
func VerifyAppliedClientHello(requested, applied *sesametls.ClientHelloSpec) error {
	if requested == nil && applied == nil {
		return nil
	}
	if requested == nil && applied != nil {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: security violation: server applied a client_hello spec that was not requested")
	}
	if requested != nil && applied == nil {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: security violation: server did not echo applied client_hello for a requested spec")
	}
	// Both non-nil: deep compare.
	if !clientHelloSpecEqual(requested, applied) {
		return status.Error(codes.FailedPrecondition,
			"sesame/rc/netconn: security violation: applied client_hello does not match requested spec")
	}
	return nil
}

// clientHelloSpecEqual performs a field-by-field comparison.
func clientHelloSpecEqual(a, b *sesametls.ClientHelloSpec) bool {
	if !int32SliceEqual(a.GetCipherSuites(), b.GetCipherSuites()) {
		return false
	}
	if !int32SliceEqual(a.GetSupportedGroups(), b.GetSupportedGroups()) {
		return false
	}
	if !int32SliceEqual(a.GetSignatureAlgorithms(), b.GetSignatureAlgorithms()) {
		return false
	}
	if !int32SliceEqual(a.GetCompressionMethods(), b.GetCompressionMethods()) {
		return false
	}
	if a.GetSessionIdLength() != b.GetSessionIdLength() {
		return false
	}
	if a.GetLegacyVersion() != b.GetLegacyVersion() {
		return false
	}
	if a.GetPadToSize() != b.GetPadToSize() {
		return false
	}
	// Extensions: compare type + body.
	ae, be := a.GetExtensions(), b.GetExtensions()
	if len(ae) != len(be) {
		return false
	}
	for i := range ae {
		if ae[i].GetType() != be[i].GetType() {
			return false
		}
		if !slices.Equal(ae[i].GetRaw(), be[i].GetRaw()) {
			return false
		}
		aAuto := ae[i].GetAuto() != nil
		bAuto := be[i].GetAuto() != nil
		if aAuto != bAuto {
			return false
		}
	}
	return true
}

func int32SliceEqual(a, b []int32) bool {
	return slices.Equal(a, b)
}
