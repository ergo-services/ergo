package dist

import (
	"bytes"
	"fmt"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"math/rand"
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

func TestDecodeDistHeaderAtomCache(t *testing.T) {
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
	cache, packet1 := link.decodeDistHeaderAtomCache(packet[2:])

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

func TestEncodeDistHeaderAtomCache(t *testing.T) {

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	writerAtomCache := make(map[etf.Atom]etf.CacheItem)
	encodingAtomCache := etf.TakeListAtomCache()
	defer etf.ReleaseListAtomCache(encodingAtomCache)

	writerAtomCache["reg"] = etf.CacheItem{ID: 1000, Encoded: false, Name: "reg"}
	writerAtomCache["call"] = etf.CacheItem{ID: 499, Encoded: false, Name: "call"}
	writerAtomCache["one_more_atom"] = etf.CacheItem{ID: 199, Encoded: true, Name: "one_more_atom"}
	writerAtomCache["yet_another_atom"] = etf.CacheItem{ID: 2, Encoded: false, Name: "yet_another_atom"}
	writerAtomCache["extra_atom"] = etf.CacheItem{ID: 10, Encoded: true, Name: "extra_atom"}
	writerAtomCache["potato"] = etf.CacheItem{ID: 2017, Encoded: true, Name: "potato"}

	// Encoded field is ignored here
	encodingAtomCache.Append(etf.CacheItem{ID: 499, Name: "call"})
	encodingAtomCache.Append(etf.CacheItem{ID: 1000, Name: "reg"})
	encodingAtomCache.Append(etf.CacheItem{ID: 199, Name: "one_more_atom"})
	encodingAtomCache.Append(etf.CacheItem{ID: 2017, Name: "potato"})

	expected := []byte{
		4, 185, 112, 0, // 4 atoms and theirs flags
		243, 4, 99, 97, 108, 108, // atom call
		232, 3, 114, 101, 103, // atom reg
		199, // atom one_more_atom, already encoded
		225, // atom potato, already encoded

	}

	l := &Link{}
	l.encodeDistHeaderAtomCache(b, writerAtomCache, encodingAtomCache)

	if !reflect.DeepEqual(b.B, expected) {
		t.Fatal("incorrect value")
	}

	b.Reset()
	encodingAtomCache.Append(etf.CacheItem{ID: 2, Name: "yet_another_atom"})

	expected = []byte{
		5, 49, 112, 8, // 5 atoms and theirs flags
		243,                      // atom call. already encoded
		232,                      // atom reg. already encoded
		199,                      // atom one_more_atom. already encoded
		225,                      // atom potato. already encoded
		2, 16, 121, 101, 116, 95, // atom yet_another_atom
		97, 110, 111, 116, 104, 101,
		114, 95, 97, 116, 111, 109,
	}
	l.encodeDistHeaderAtomCache(b, writerAtomCache, encodingAtomCache)

	if !reflect.DeepEqual(b.B, expected) {
		t.Fatal("incorrect value", b.B)
	}
}

func BenchmarkDecodeDistHeaderAtomCache(b *testing.B) {
	link := &Link{}
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
		link.decodeDistHeaderAtomCache(packet[2:])
	}
}

func BenchmarkEncodeDistHeaderAtomCache(b *testing.B) {
	link := &Link{}
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	writerAtomCache := make(map[etf.Atom]etf.CacheItem)
	encodingAtomCache := etf.TakeListAtomCache()
	defer etf.ReleaseListAtomCache(encodingAtomCache)

	writerAtomCache["reg"] = etf.CacheItem{ID: 1000, Encoded: false, Name: "reg"}
	writerAtomCache["call"] = etf.CacheItem{ID: 499, Encoded: false, Name: "call"}
	writerAtomCache["one_more_atom"] = etf.CacheItem{ID: 199, Encoded: true, Name: "one_more_atom"}
	writerAtomCache["yet_another_atom"] = etf.CacheItem{ID: 2, Encoded: false, Name: "yet_another_atom"}
	writerAtomCache["extra_atom"] = etf.CacheItem{ID: 10, Encoded: true, Name: "extra_atom"}
	writerAtomCache["potato"] = etf.CacheItem{ID: 2017, Encoded: true, Name: "potato"}

	// Encoded field is ignored here
	encodingAtomCache.Append(etf.CacheItem{ID: 499, Name: "call"})
	encodingAtomCache.Append(etf.CacheItem{ID: 1000, Name: "reg"})
	encodingAtomCache.Append(etf.CacheItem{ID: 199, Name: "one_more_atom"})
	encodingAtomCache.Append(etf.CacheItem{ID: 2017, Name: "potato"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		link.encodeDistHeaderAtomCache(buf, writerAtomCache, encodingAtomCache)
	}
}

func TestDecodeFragment(t *testing.T) {
	link := &Link{}

	link.checkCleanTimeout = 50 * time.Millisecond
	link.checkCleanDeadline = 150 * time.Millisecond

	// decode fragment with fragmentID=0 should return error
	fragment0 := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3}
	if _, e := link.decodeFragment(fragment0, true); e == nil {
		t.Fatal("should be error here")
	}

	fragment1 := []byte{0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 3, 1, 2, 3}
	fragment2 := []byte{0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 2, 4, 5, 6}
	fragment3 := []byte{0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1, 7, 8, 9}

	expected := []byte{68, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	// add first fragment
	if x, e := link.decodeFragment(fragment1, true); x != nil || e != nil {
		t.Fatal("should be nil here", e)
	}
	// add second one
	if x, e := link.decodeFragment(fragment2, false); x != nil || e != nil {
		t.Fatal("should be nil here", e)
	}
	// add the last one. should return *lib.Buffer with assembled packet
	if x, e := link.decodeFragment(fragment3, false); x == nil || e != nil {
		t.Fatal("shouldn't be nil here", e)
	} else {
		// x should be *lib.Buffer
		if !reflect.DeepEqual(expected, x.B) {
			t.Fatal("exp:", expected, "got:", x.B)
		}
		lib.ReleaseBuffer(x)

		// map of the fragments should be empty here
		if len(link.fragments) > 0 {
			t.Fatal("fragments should be empty")
		}
	}

	link.checkCleanTimeout = 50 * time.Millisecond
	link.checkCleanDeadline = 150 * time.Millisecond
	// test lost fragment
	// add the first fragment and wait 160ms
	if x, e := link.decodeFragment(fragment1, true); x != nil || e != nil {
		t.Fatal("should be nil here", e)
	}
	if len(link.fragments) == 0 {
		t.Fatal("fragments should have a record ")
	}
	// check and clean process should remove this record
	time.Sleep(360 * time.Millisecond)

	// map of the fragments should be empty here
	if len(link.fragments) > 0 {
		t.Fatal("fragments should be empty")
	}

	link.checkCleanTimeout = 0
	link.checkCleanDeadline = 0
	fragments := [][]byte{
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 9, 1, 2, 3},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 8, 4, 5, 6},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 7, 7, 8, 9},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 6, 10, 11, 12},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 5, 13, 14, 15},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 4, 16, 17, 18},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 3, 19, 20, 21},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 2, 22, 23, 24},
		[]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 1, 25, 26, 27},
	}
	expected = []byte{68, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}

	fragmentsReverse := make([][]byte, len(fragments))
	l := len(fragments)
	for i := 0; i < l; i++ {
		fragmentsReverse[l-i-1] = fragments[i]
	}

	var result *lib.Buffer
	var e error
	var first bool
	for i := range fragmentsReverse {
		first = false
		if fragmentsReverse[i][15] == byte(l) {
			first = true
		}
		if result, e = link.decodeFragment(fragmentsReverse[i], first); e != nil {
			t.Fatal(e)
		}

	}
	if result == nil {
		t.Fatal("got nil result")
	}
	if !reflect.DeepEqual(expected, result.B) {
		t.Fatal("exp:", expected, "got:", result.B[:len(expected)])
	}
	// map of the fragments should be empty here
	if len(link.fragments) > 0 {
		t.Fatal("fragments should be empty")
	}

	// reshuffling 100 times
	for k := 0; k < 100; k++ {
		result = nil
		fragmentsShuffle := make([][]byte, l)
		rand.Seed(time.Now().UnixNano())
		for i, v := range rand.Perm(l) {
			fragmentsShuffle[v] = fragments[i]
		}

		for i := range fragmentsShuffle {
			first = false
			if fragmentsShuffle[i][15] == byte(l) {
				first = true
			}
			if result, e = link.decodeFragment(fragmentsShuffle[i], first); e != nil {
				t.Fatal(e)
			}

		}
		if result == nil {
			t.Fatal("got nil result")
		}
		if !reflect.DeepEqual(expected, result.B) {
			t.Fatal("exp:", expected, "got:", result.B[:len(expected)])
		}
	}
}
