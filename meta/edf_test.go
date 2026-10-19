package meta_test

import (
	"bytes"
	"encoding/hex"
	"net/netip"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/net/edf"
)

var testAlias = gen.Alias{Node: "node@localhost", Creation: 1234567890, ID: [3]uint64{1, 2, 3}}

func encodeDecode(t *testing.T, value any) any {
	t.Helper()
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	if err := edf.Encode(value, b, edf.Options{}); err != nil {
		t.Fatalf("encode %T: %v", value, err)
	}
	decoded, tail, err := edf.Decode(b.B, edf.Options{})
	if err != nil {
		t.Fatalf("decode %T: %v", value, err)
	}
	if len(tail) > 0 {
		t.Fatalf("decode %T left %d byte(s) unread", value, len(tail))
	}
	return decoded
}

// the meta messages and the address type they carry must be known to the
// network out of the box, otherwise a remote process cannot use a socket
func TestEDFMetaTypesRegistered(t *testing.T) {
	names := []string{
		"#ergo.services/ergo/meta/MessageTCPConnect",
		"#ergo.services/ergo/meta/MessageTCPDisconnect",
		"#ergo.services/ergo/meta/MessageTCP",
		"#ergo.services/ergo/meta/MessageUDP",
		"#net/netip/AddrPort",
	}
	r := edf.NewRegistry()
	for _, name := range names {
		if _, found := r.LookupType(name); found == false {
			t.Errorf("type %s is not registered", name)
		}
	}
}

func TestEDFMessageUDPRoundTrip(t *testing.T) {
	addrs := []netip.AddrPort{
		netip.MustParseAddrPort("127.0.0.1:5000"),
		netip.MustParseAddrPort("[::1]:5000"),
		netip.MustParseAddrPort("[fe80::1%eth0]:5000"),
		{},
	}

	for _, addr := range addrs {
		original := meta.MessageUDP{ID: testAlias, Addr: addr, Data: []byte("payload")}
		decoded, ok := encodeDecode(t, original).(meta.MessageUDP)
		if ok == false {
			t.Fatalf("decoded value of %v is not MessageUDP", addr)
		}
		if decoded.ID != original.ID {
			t.Errorf("ID = %s, want %s", decoded.ID, original.ID)
		}
		if decoded.Addr != original.Addr {
			t.Errorf("Addr = %v, want %v", decoded.Addr, original.Addr)
		}
		if decoded.Addr.Addr().Zone() != addr.Addr().Zone() {
			t.Errorf("zone = %q, want %q", decoded.Addr.Addr().Zone(), addr.Addr().Zone())
		}
		if bytes.Equal(decoded.Data, original.Data) == false {
			t.Errorf("Data = %q, want %q", decoded.Data, original.Data)
		}
	}
}

func TestEDFMessageTCPConnectRoundTrip(t *testing.T) {
	addrs := [][2]netip.AddrPort{
		{netip.MustParseAddrPort("10.0.0.1:443"), netip.MustParseAddrPort("10.0.0.2:51515")},
		{netip.MustParseAddrPort("[2001:db8::1]:443"), netip.MustParseAddrPort("[2001:db8::2]:51515")},
		{netip.MustParseAddrPort("[fe80::1%eth0]:443"), netip.MustParseAddrPort("[fe80::2%eth0]:51515")},
		{{}, {}},
	}

	for _, pair := range addrs {
		original := meta.MessageTCPConnect{ID: testAlias, RemoteAddr: pair[0], LocalAddr: pair[1]}
		decoded, ok := encodeDecode(t, original).(meta.MessageTCPConnect)
		if ok == false {
			t.Fatalf("decoded value of %v is not MessageTCPConnect", pair)
		}
		if decoded != original {
			t.Errorf("decoded = %+v, want %+v", decoded, original)
		}
		if decoded.RemoteAddr.Addr().Zone() != pair[0].Addr().Zone() {
			t.Errorf("remote zone = %q, want %q", decoded.RemoteAddr.Addr().Zone(), pair[0].Addr().Zone())
		}
	}
}

func TestEDFMessageTCPRoundTrip(t *testing.T) {
	original := meta.MessageTCP{ID: testAlias, Data: []byte{1, 2, 3, 4}}
	decoded, ok := encodeDecode(t, original).(meta.MessageTCP)
	if ok == false {
		t.Fatal("decoded value is not MessageTCP")
	}
	if decoded.ID != original.ID || bytes.Equal(decoded.Data, original.Data) == false {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}

	disconnect := meta.MessageTCPDisconnect{ID: testAlias}
	if decoded, ok := encodeDecode(t, disconnect).(meta.MessageTCPDisconnect); ok == false || decoded != disconnect {
		t.Errorf("decoded = %+v, want %+v", decoded, disconnect)
	}
}

// the wire format of MessageUDP is part of the protocol: other implementations
// (ergo-rs) encode the same bytes. Changing it breaks them.
func TestEDFMessageUDPGolden(t *testing.T) {
	golden := []struct {
		addr string
		hex  string
	}{
		{
			"127.0.0.1:5000",
			"830023236572676f2e73657276696365732f6572676f2f6d6574612f4d6573736167" +
				"6555445000" + "0e6e6f6465406c6f63616c686f7374" +
				"00000000499602d2000000000000000100000000000000020000000000000003" +
				"000000067f00000188130000000178",
		},
		{
			"[::1]:5000",
			"830023236572676f2e73657276696365732f6572676f2f6d6574612f4d6573736167" +
				"6555445000" + "0e6e6f6465406c6f63616c686f7374" +
				"00000000499602d2000000000000000100000000000000020000000000000003" +
				"0000001200000000000000000000000000000001" + "88130000000178",
		},
	}

	for _, g := range golden {
		m := meta.MessageUDP{ID: testAlias, Addr: netip.MustParseAddrPort(g.addr), Data: []byte("x")}
		b := lib.TakeBuffer()
		if err := edf.Encode(m, b, edf.Options{}); err != nil {
			lib.ReleaseBuffer(b)
			t.Fatalf("encode %s: %v", g.addr, err)
		}
		got := hex.EncodeToString(b.B)
		lib.ReleaseBuffer(b)
		if got != g.hex {
			t.Errorf("meta.MessageUDP{%s}\n got %s\nwant %s", g.addr, got, g.hex)
		}
	}
}
