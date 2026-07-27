package edf

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestNetworkProxyFlagsRoundTrip guards #256: NetworkProxyFlags.MarshalEDF/UnmarshalEDF
// were no-ops, so every flag was silently dropped on the wire. A mixed set of bits must
// now survive an Encode/Decode round-trip (mixed values also catch swapped bit positions).
func TestNetworkProxyFlagsRoundTrip(t *testing.T) {
	original := gen.NetworkProxyFlags{
		Enable:                       true,
		EnableRemoteSpawn:            true,
		EnableRemoteApplicationStart: false,
		EnableEncryption:             true,
		EnableImportantDelivery:      false,
	}

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	if err := Encode(original, buf, Options{}); err != nil {
		t.Fatalf("Encode: %s", err)
	}

	decoded, _, err := Decode(buf.B, Options{})
	if err != nil {
		t.Fatalf("Decode: %s", err)
	}

	got, ok := decoded.(gen.NetworkProxyFlags)
	if ok == false {
		t.Fatalf("expected gen.NetworkProxyFlags, got %T", decoded)
	}
	if got != original {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, original)
	}
}
