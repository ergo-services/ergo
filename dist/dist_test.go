package dist

import (
	"bytes"
	"testing"
)

func Test_compose_SEND_NAME(t *testing.T) {
	var b bytes.Buffer
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
	link.composeName(&b)
	shouldBe := []byte{}

	if !bytes.Equal(b.Bytes(), shouldBe) {
		t.Fatal("malform value")
	}

}

func Test_compose_SEND_CHALLENGE_REPLY(t *testing.T) {

}
