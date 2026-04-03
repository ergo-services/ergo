package gen

import (
	"testing"
	"time"
)

func TestSamplerAlways(t *testing.T) {
	s := TracingSamplerAlways
	for i := 0; i < 1000; i++ {
		if s.Sample() == false {
			t.Fatal("TracingSamplerAlways returned false")
		}
	}
	if s.String() != "always" {
		t.Fatalf("expected 'always', got %q", s.String())
	}
}

func TestSamplerDisable(t *testing.T) {
	s := TracingSamplerDisable
	for i := 0; i < 1000; i++ {
		if s.Sample() == true {
			t.Fatal("TracingSamplerDisable returned true")
		}
	}
	if s.String() != "disable" {
		t.Fatalf("expected 'disable', got %q", s.String())
	}
}

func TestSamplerRatioZero(t *testing.T) {
	s := TracingSamplerRatio(0)
	if s != TracingSamplerDisable {
		t.Fatal("Ratio(0) should return TracingSamplerDisable")
	}
}

func TestSamplerRatioOne(t *testing.T) {
	s := TracingSamplerRatio(1)
	if s != TracingSamplerAlways {
		t.Fatal("Ratio(1) should return TracingSamplerAlways")
	}
}

func TestSamplerRatio(t *testing.T) {
	s := TracingSamplerRatio(0.5)
	count := 0
	total := 10000
	for i := 0; i < total; i++ {
		if s.Sample() {
			count++
		}
	}
	// expect ~50% with tolerance
	ratio := float64(count) / float64(total)
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("expected ~50%% samples, got %.2f%%", ratio*100)
	}
	if s.String() != "ratio(0.5)" {
		t.Fatalf("expected 'ratio(0.5)', got %q", s.String())
	}
}

func TestSamplerRatioSmall(t *testing.T) {
	s := TracingSamplerRatio(0.01)
	count := 0
	total := 100000
	for i := 0; i < total; i++ {
		if s.Sample() {
			count++
		}
	}
	ratio := float64(count) / float64(total)
	if ratio < 0.005 || ratio > 0.015 {
		t.Fatalf("expected ~1%% samples, got %.2f%%", ratio*100)
	}
}

func TestSamplerRateLimit(t *testing.T) {
	s := TracingSamplerRateLimit(10)
	count := 0
	for i := 0; i < 100; i++ {
		if s.Sample() {
			count++
		}
	}
	if count != 10 {
		t.Fatalf("expected 10 samples in first burst, got %d", count)
	}
	if s.String() != "rate_limit(10/s)" {
		t.Fatalf("expected 'rate_limit(10/s)', got %q", s.String())
	}
}

func TestSamplerRateLimitRefill(t *testing.T) {
	s := TracingSamplerRateLimit(5)
	// exhaust tokens
	for i := 0; i < 10; i++ {
		s.Sample()
	}
	// wait for refill
	time.Sleep(1100 * time.Millisecond)
	count := 0
	for i := 0; i < 10; i++ {
		if s.Sample() {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("expected 5 samples after refill, got %d", count)
	}
}

func TestSamplerRateLimitZero(t *testing.T) {
	s := TracingSamplerRateLimit(0)
	if s != TracingSamplerDisable {
		t.Fatal("RateLimit(0) should return TracingSamplerDisable")
	}
}
