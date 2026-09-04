package meta

import (
	"testing"
	"time"

	"ergo.services/ergo/testing/mock"
)

// metaSink returns a dumb gen.MetaProcess mock whose Send pushes every outgoing
// message to the returned buffered channel, so a lifecycle test can read the
// egress sequence in order.
func metaSink() (*mock.Meta, chan any) {
	ch := make(chan any, 64)
	mp := mock.NewMeta()
	mp.OnSend(func(to any, message any) error {
		ch <- message
		return nil
	})
	return mp, ch
}

// recvMsg waits up to a second for the next message and asserts its type.
func recvMsg[T any](t *testing.T, ch chan any) T {
	t.Helper()
	select {
	case v := <-ch:
		got, ok := v.(T)
		if ok == false {
			t.Fatalf("got %T, want %T", v, got)
		}
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a message")
	}
	var zero T
	return zero
}

// recvNone asserts that no message arrives within a short window.
func recvNone(t *testing.T, ch chan any) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("unexpected message: %#v", v)
	case <-time.After(100 * time.Millisecond):
	}
}
