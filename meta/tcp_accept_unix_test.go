//go:build unix

package meta

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestTCPServerAcceptExhausted runs out of descriptors while a client
// connects: the server stays, waits and accepts once there is room. It runs
// again in a new OS process with 128 descriptors: they are its own.
func TestTCPServerAcceptExhausted(t *testing.T) {
	if os.Getenv("ERGO_TEST_ACCEPT_EXHAUSTED") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTCPServerAcceptExhausted$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "ERGO_TEST_ACCEPT_EXHAUSTED=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s\n%s", err, out)
		}
		return
	}

	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	limit.Cur = 128
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}

	mb, err := CreateTCPServer(TCPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	srv := mb.(*tcpserver)
	addr := srv.listener.Addr().String()
	mp, _ := metaSink()
	spawned := make(chan struct{}, 4)
	mp.OnSpawn(func(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
		spawned <- struct{}{}
		return gen.Alias{}, nil
	})
	srv.Init(mp)
	done := make(chan error, 1)
	go func() { done <- srv.Start() }()

	var held []*os.File
	for {
		f, err := os.Open(os.DevNull)
		if err != nil {
			if lib.AcceptExhausted(err) == false {
				t.Fatal(err)
			}
			break
		}
		held = append(held, f)
	}
	held[len(held)-1].Close()
	held = held[:len(held)-1]
	first, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	select {
	case err := <-done:
		t.Fatalf("the server ended on an exhausted accept: %v", err)
	case <-spawned:
		t.Fatal("accepted with no descriptor left")
	case <-time.After(200 * time.Millisecond):
	}

	for _, f := range held[len(held)-16:] {
		f.Close()
	}
	second, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	select {
	case err := <-done:
		t.Fatalf("the server ended: %v", err)
	case <-spawned:
	case <-time.After(5 * time.Second):
		t.Fatal("no accept once there is room")
	}
	srv.listener.Close()
	if err := <-done; errors.Is(err, net.ErrClosed) == false {
		t.Fatalf("the end of the server: %v", err)
	}
}
