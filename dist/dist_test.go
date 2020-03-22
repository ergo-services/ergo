package dist

import (
	"bytes"
	"fmt"
	"github.com/halturin/ergo/lib"
	"net"
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
	link := &Link{
		Name:   "testName",
		Cookie: "testCookie",
		Hidden: false,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
			FRAGMENTS,
		),

		version: 5,
	}
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	link.composeName(b)
	shouldBe := []byte{}

	if !bytes.Equal(b.B, shouldBe) {
		t.Fatal("malform value")
	}

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
