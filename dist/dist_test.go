package dist

import (
	"bytes"
	"fmt"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestLinkRead(t *testing.T) {

	server, client := net.Pipe()
	defer func() {
		server.Close()
		client.Close()
	}()

	link := Link{
		conn: server,
	}

	go client.Write([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0})

	// read keepalive answer on a client side
	go func() {
		bb := make([]byte, 10)
		for {
			_, e := client.Read(bb)
			if e != nil {
				return
			}
		}
	}()

	c := make(chan bool)
	b := lib.TakeBuffer()
	go func() {
		link.Read(b)
		close(c)
	}()
	select {
	case <-c:
		fmt.Println("OK", b.B)
	case <-time.After(1000 * time.Millisecond):
		t.Fatal("incorrect")
	}

}

func TestComposeName(t *testing.T) {
	//link := &Link{
	//	Name:   "testName",
	//	Cookie: "testCookie",
	//	Hidden: false,

	//	flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
	//		EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES,
	//		DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
	//		SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
	//		FRAGMENTS,
	//	),

	//	version: 5,
	//}
	//b := lib.TakeBuffer()
	//defer lib.ReleaseBuffer(b)
	//link.composeName(b)
	//shouldBe := []byte{}

	//if !bytes.Equal(b.B, shouldBe) {
	//	t.Fatal("malform value")
	//}

}

func TestReadName(t *testing.T) {

}

func TestComposeStatus(t *testing.T) {

}

func TestComposeChallenge(t *testing.T) {

}

func TestReadChallenge(t *testing.T) {

}

func TestValidateChallengeReply(t *testing.T) {

}

func TestComposeChallengeAck(t *testing.T) {

}

func TestComposeChalleneReply(t *testing.T) {

}

func TestValidateChallengeAck(t *testing.T) {

}

func TestReadDistHeaderAtomCache(t *testing.T) {
	link := Link{}
	link.cacheIn[1034] = "atom1"
	link.cacheIn[5] = "atom2"
	packet := []byte{
		131, 68, // start dist header
		5, 4, 137, 9, // 5 atoms and theirs flags
		10, 5, // already cached atom ids
		236, 3, 114, 101, 103, // atom 'reg'
		9, 4, 99, 97, 108, 108, //atom 'call'
		238, 13, 115, 101, 116, 95, 103, 101, 116, 95, 115, 116, 97, 116, 101, // atom 'set_get_state'
		104, 4, 97, 6, 103, 82, 0, 0, 0, 0, 85, 0, 0, 0, 0, 2, 82, 1, 82, 2, // message...
		104, 3, 82, 3, 103, 82, 0, 0, 0, 0, 245, 0, 0, 0, 2, 2,
		104, 2, 82, 4, 109, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}

	cacheExpected := []etf.Atom{"atom1", "atom2", "reg", "call", "set_get_state"}
	cacheInExpected := link.cacheIn
	cacheInExpected[492] = "reg"
	cacheInExpected[9] = "call"
	cacheInExpected[494] = "set_get_state"

	packetExpected := packet[34:]
	cache, packet1 := link.readDistHeaderAtomCache(packet[2:])

	if !bytes.Equal(packet1, packetExpected) {
		t.Fatal("incorrect packet")
	}

	if !reflect.DeepEqual(link.cacheIn, cacheInExpected) {
		t.Fatal("incorrect cacheIn")
	}

	if !reflect.DeepEqual(cache, cacheExpected) {
		t.Fatal("incorrect cache", cache)
	}
}

func BenchmarkReadDistHeaderAtomCache(b *testing.B) {
	link := Link{}
	packet := []byte{
		131, 68, // start dist header
		5, 4, 137, 9, // 5 atoms and theirs flags
		10, 5, // already cached atom ids
		236, 3, 114, 101, 103, // atom 'reg'
		9, 4, 99, 97, 108, 108, //atom 'call'
		238, 13, 115, 101, 116, 95, 103, 101, 116, 95, 115, 116, 97, 116, 101, // atom 'set_get_state'
		104, 4, 97, 6, 103, 82, 0, 0, 0, 0, 85, 0, 0, 0, 0, 2, 82, 1, 82, 2, // message...
		104, 3, 82, 3, 103, 82, 0, 0, 0, 0, 245, 0, 0, 0, 2, 2,
		104, 2, 82, 4, 109, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		link.readDistHeaderAtomCache(packet[2:])
	}
}
