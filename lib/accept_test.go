package lib

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestAcceptExhausted(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM} {
		err := &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", errno)}
		if AcceptExhausted(err) == false {
			t.Fatalf("%s is not exhausted", err)
		}
	}
	for _, err := range []error{net.ErrClosed, fmt.Errorf("accept: %w", syscall.EINVAL)} {
		if AcceptExhausted(err) {
			t.Fatalf("%s is exhausted", err)
		}
	}
}

func TestAcceptBackoff(t *testing.T) {
	var b AcceptBackoff
	want := []time.Duration{5, 10, 20, 40, 80, 160, 320, 640, 1000, 1000}
	for i, w := range want {
		if d := b.Next(); d != w*time.Millisecond {
			t.Fatalf("wait %d: %s, want %s", i, d, w*time.Millisecond)
		}
	}
	b.Reset()
	if d := b.Next(); d != 5*time.Millisecond {
		t.Fatalf("after reset: %s", d)
	}
}
