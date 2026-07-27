package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestWebResponseWriterWorkerWins: once the worker has written, a deadline timeout
// must lose the claim and add no 504 to the response.
func TestWebResponseWriterWorkerWins(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rw := &webResponseWriter{ResponseWriter: rec, ctx: ctx}

	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write([]byte("body")); err != nil {
		t.Fatal(err)
	}
	rw.timeout() // must lose: the worker already owns the writer

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "body" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "body")
	}
}

// TestWebResponseWriterTimeoutWins: once the timeout has claimed the writer, a late
// worker write must drop and not leak into the 504 response.
func TestWebResponseWriterTimeoutWins(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rw := &webResponseWriter{ResponseWriter: rec, ctx: ctx}

	rw.timeout()
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code = %d, want 504", rec.Code)
	}
	body := rec.Body.String()

	if n, _ := rw.Write([]byte("late")); n != 0 {
		t.Fatalf("late write reached the response: n=%d", n)
	}
	rw.WriteHeader(http.StatusInternalServerError) // must be a no-op
	if rec.Body.String() != body {
		t.Fatalf("late write leaked into response: %q", rec.Body.String())
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("late WriteHeader changed status: %d", rec.Code)
	}
}

// TestWebResponseWriterFlushGating: a Flush before any write must neither touch the
// underlying flusher nor claim the writer; after a worker write it must flush.
func TestWebResponseWriterFlushGating(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rw := &webResponseWriter{ResponseWriter: rec, ctx: ctx}

	rw.Flush()
	if rw.state.Load() != webWriterUnclaimed {
		t.Fatal("Flush claimed the writer before any write")
	}
	if rec.Flushed {
		t.Fatal("Flush reached the underlying flusher before any write")
	}
	rw.timeout() // must still be free to win
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code = %d, want 504 (Flush must not steal the claim)", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	rw2 := &webResponseWriter{ResponseWriter: rec2, ctx: ctx2}
	rw2.Write([]byte("x"))
	rw2.Flush()
	if rec2.Flushed == false {
		t.Fatal("Flush did not reach the underlying flusher after a worker write")
	}
}

// TestWebResponseWriterMutualExclusion: a worker write racing the deadline 504 on the
// same writer resolves to exactly one owner, so only one goroutine ever touches the
// underlying writer. Run with -race to catch a concurrent underlying write.
func TestWebResponseWriterMutualExclusion(t *testing.T) {
	for i := 0; i < 2000; i++ {
		rec := httptest.NewRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		rw := &webResponseWriter{ResponseWriter: rec, ctx: ctx}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte("W"))
		}()
		go func() {
			defer wg.Done()
			rw.timeout()
		}()
		wg.Wait()
		cancel()

		workerWon := rec.Code == http.StatusOK && rec.Body.String() == "W"
		timeoutWon := rec.Code == http.StatusGatewayTimeout && rec.Body.String() == "Gateway timeout\n"
		if workerWon == timeoutWon {
			t.Fatalf("iter %d: not a single winner: code=%d body=%q", i, rec.Code, rec.Body.String())
		}
	}
}
