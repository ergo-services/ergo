package decimal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

// The checks compare with ==: NaN has one representation.

func TestNaNPropagates(t *testing.T) {
	x := RequireFromString("12.5")
	cases := []struct {
		name string
		got  func() Decimal
	}{
		{"add", func() Decimal { return NaN.Add(x) }},
		{"add rhs", func() Decimal { return x.Add(NaN) }},
		{"add both", func() Decimal { return NaN.Add(NaN) }},
		{"sub", func() Decimal { return NaN.Sub(x) }},
		{"sub rhs", func() Decimal { return x.Sub(NaN) }},
		{"sub both", func() Decimal { return NaN.Sub(NaN) }},
		{"mul", func() Decimal { return NaN.Mul(x) }},
		{"mul rhs", func() Decimal { return x.Mul(NaN) }},
		{"mul zero", func() Decimal { return Zero.Mul(NaN) }},
		{"div", func() Decimal { return NaN.Div(x) }},
		{"div rhs", func() Decimal { return x.Div(NaN) }},
		{"div zero by NaN", func() Decimal { return Zero.Div(NaN) }},
		{"div NaN by zero", func() Decimal { return NaN.Div(Zero) }},
		{"div round", func() Decimal { return NaN.DivRound(x, 4) }},
		{"mod", func() Decimal { return NaN.Mod(x) }},
		{"mod rhs", func() Decimal { return x.Mod(NaN) }},
		{"shift", func() Decimal { return NaN.Shift(3) }},
		{"shift down", func() Decimal { return NaN.Shift(-3) }},
		{"round", func() Decimal { return NaN.Round(2) }},
		{"round to a scale past MaxScale", func() Decimal { return NaN.Round(50) }},
		{"round to the NaN scale code", func() Decimal { return NaN.Round(63) }},
		{"round integer part", func() Decimal { return NaN.Round(-2) }},
		{"round bank", func() Decimal { return NaN.RoundBank(2) }},
		{"truncate", func() Decimal { return NaN.Truncate(2) }},
		{"floor", func() Decimal { return NaN.Floor() }},
		{"ceil", func() Decimal { return NaN.Ceil() }},
		{"neg", func() Decimal { return NaN.Neg() }},
		{"abs", func() Decimal { return NaN.Abs() }},
		{"canonical", func() Decimal { return NaN.Canonical() }},
		{"sum", func() Decimal { return Sum(x, NaN, x) }},
		{"avg", func() Decimal { return Avg(x, NaN) }},
		{"min", func() Decimal { return Min(x, NaN, One) }},
		{"min NaN first", func() Decimal { return Min(NaN, x) }},
		{"max", func() Decimal { return Max(x, NaN, One) }},
		{"max NaN first", func() Decimal { return Max(NaN, x) }},
		{"max NaN last", func() Decimal { return Max(x, One, NaN) }},
	}
	for _, c := range cases {
		if got := c.got(); got != NaN {
			t.Errorf("%s = %#v, want NaN", c.name, got)
		}
	}

	checked := []struct {
		name string
		got  func() (Decimal, error)
	}{
		{"checked add", func() (Decimal, error) { return x.CheckedAdd(NaN) }},
		{"checked sub", func() (Decimal, error) { return NaN.CheckedSub(x) }},
		{"checked mul", func() (Decimal, error) { return x.CheckedMul(NaN) }},
		{"checked div", func() (Decimal, error) { return NaN.CheckedDiv(Zero) }},
		{"quo round", func() (Decimal, error) { return x.QuoRound(NaN, 2, RoundFloor) }},
		{"rescale", func() (Decimal, error) { return NaN.Rescale(4) }},
	}
	for _, c := range checked {
		if got, err := c.got(); got != NaN || err != nil {
			t.Errorf("%s = %#v, %v, want NaN, nil", c.name, got, err)
		}
	}

	if q, r := NaN.QuoRem(x, 2); q != NaN || r != NaN {
		t.Errorf("quo rem = %#v, %#v, want NaN, NaN", q, r)
	}
}

func TestNaNOrdering(t *testing.T) {
	low := RequireFromString("-1e30")
	if c := NaN.Cmp(low); c != -1 {
		t.Errorf("NaN.Cmp(-1e30) = %d, want -1", c)
	}
	if c := low.Cmp(NaN); c != 1 {
		t.Errorf("-1e30.Cmp(NaN) = %d, want 1", c)
	}
	if c := NaN.Cmp(NaN); c != 0 {
		t.Errorf("NaN.Cmp(NaN) = %d, want 0", c)
	}
	if NaN.Equal(NaN) == false {
		t.Error("NaN must equal NaN")
	}
	if NaN.Equal(Zero) || Zero.Equal(NaN) {
		t.Error("NaN must not equal zero")
	}
	if NaN.LessThan(Zero) == false || NaN.GreaterThan(low) {
		t.Error("NaN must sort below every number")
	}

	sorted := []Decimal{One, NaN, low, Zero}
	slices.SortFunc(sorted, Decimal.Cmp)
	if want := []Decimal{NaN, low, Zero, One}; slices.Equal(sorted, want) == false {
		t.Errorf("sorted = %v, want %v", sorted, want)
	}
}

func TestNaNPredicates(t *testing.T) {
	for _, d := range []Decimal{Zero, One, RequireFromString("0.000"), RequireFromString("-7.25")} {
		if d.IsNaN() {
			t.Errorf("%s reported as NaN", d)
		}
	}
	if NaN.IsNaN() == false {
		t.Error("NaN not reported as NaN")
	}
	if NaN.IsZero() || NaN.IsPositive() || NaN.IsNegative() || NaN.IsInteger() {
		t.Error("NaN is neither zero, positive, negative nor an integer")
	}
	if NaN.Sign() != 0 || NaN.Scale() != 0 || NaN.Exponent() != 0 || NaN.NumDigits() != 0 {
		t.Errorf("sign %d scale %d exponent %d digits %d, want all 0",
			NaN.Sign(), NaN.Scale(), NaN.Exponent(), NaN.NumDigits())
	}
}

func TestNaNConversions(t *testing.T) {
	if f, exact := NaN.Float64(); math.IsNaN(f) == false || exact == false {
		t.Errorf("Float64 = %v, %v, want NaN, true", f, exact)
	}
	if f := NaN.InexactFloat64(); math.IsNaN(f) == false {
		t.Errorf("InexactFloat64 = %v, want NaN", f)
	}

	panics := map[string]func(){
		"BigInt":           func() { NaN.BigInt() },
		"Coefficient":      func() { NaN.Coefficient() },
		"Rat":              func() { NaN.Rat() },
		"IntPart":          func() { NaN.IntPart() },
		"CoefficientInt64": func() { NaN.CoefficientInt64() },
	}
	for name, f := range panics {
		func() {
			defer func() {
				if v := recover(); v != ErrNaN {
					t.Errorf("%s panicked with %v, want ErrNaN", name, v)
				}
			}()
			f()
		}()
	}
}

func TestNaNConstructorsReject(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "-NaN"} {
		if d, err := NewFromString(s); err == nil {
			t.Errorf("NewFromString(%q) = %s, want an error", s, d)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("NewFromFloat(NaN) must panic")
		}
	}()
	NewFromFloat(math.NaN())
}

func TestNaNText(t *testing.T) {
	for name, got := range map[string]string{
		"String":         NaN.String(),
		"StringScaled":   NaN.StringScaled(),
		"StringFixed":    NaN.StringFixed(2),
		"StringFixed 40": NaN.StringFixed(40),
		"AppendString":   string(NaN.AppendString(nil)),
		"Sprint":         fmt.Sprint(NaN),
	} {
		if got != "NaN" {
			t.Errorf("%s = %q, want NaN", name, got)
		}
	}
	if got := NaN.GoString(); got != "decimal.NaN" {
		t.Errorf("GoString = %q, want decimal.NaN", got)
	}

	b, _ := NaN.MarshalText()
	if string(b) != "NaN" {
		t.Errorf("MarshalText = %q", b)
	}
	for _, s := range []string{"NaN", "nan", "NAN"} {
		var d Decimal
		if err := d.UnmarshalText([]byte(s)); err != nil || d != NaN {
			t.Errorf("UnmarshalText(%q) = %#v, %v, want NaN", s, d, err)
		}
	}
	for _, s := range []string{"-NaN", "+NaN", "NaNx", "Na"} {
		var d Decimal
		if err := d.UnmarshalText([]byte(s)); err == nil {
			t.Errorf("UnmarshalText(%q) = %#v, want an error", s, d)
		}
	}
}

func TestNaNJSON(t *testing.T) {
	b, err := json.Marshal(NaN)
	if err != nil || string(b) != `"NaN"` {
		t.Fatalf("Marshal = %s, %v", b, err)
	}
	var d Decimal
	if err := json.Unmarshal(b, &d); err != nil || d != NaN {
		t.Errorf("Unmarshal = %#v, %v, want NaN", d, err)
	}

	b, err = json.Marshal(NewNullDecimal(NaN))
	if err != nil || string(b) != `"NaN"` {
		t.Fatalf("NullDecimal Marshal = %s, %v", b, err)
	}
	var n NullDecimal
	if err := json.Unmarshal(b, &n); err != nil || n.Valid == false || n.Decimal != NaN {
		t.Errorf("NullDecimal Unmarshal = %#v, %v, want a valid NaN", n, err)
	}
}

func TestNaNSQL(t *testing.T) {
	v, err := NaN.Value()
	if err != nil || v != "NaN" {
		t.Errorf("Value = %v, %v, want NaN", v, err)
	}
	for _, src := range []any{"NaN", []byte("NaN"), math.NaN(), float32(math.NaN()), NaN} {
		d := One
		if err := d.Scan(src); err != nil || d != NaN {
			t.Errorf("Scan(%#v) = %#v, %v, want NaN", src, d, err)
		}
	}
}

func TestNaNWire(t *testing.T) {
	b, err := NaN.MarshalBinary()
	if err != nil || bytes.Equal(b, []byte{0x3f, 0x00}) == false {
		t.Fatalf("MarshalBinary = % x, %v, want 3f 00", b, err)
	}
	if n := NaN.WireLen(); n != 2 {
		t.Errorf("WireLen = %d, want 2", n)
	}
	var w [18]byte
	if n := NaN.PutWire(w[:]); n != 2 || w[0] != 0x3f || w[1] != 0 {
		t.Errorf("PutWire = %d, % x", n, w[:n])
	}
	var d Decimal
	if err := d.UnmarshalBinary(b); err != nil || d != NaN {
		t.Errorf("UnmarshalBinary = %#v, %v, want NaN", d, err)
	}

	var buf bytes.Buffer
	if err := NaN.MarshalEDF(&buf); err != nil {
		t.Fatal(err)
	}
	d = One
	if err := d.UnmarshalEDF(buf.Bytes()); err != nil || d != NaN {
		t.Errorf("EDF round trip = %#v, %v, want NaN", d, err)
	}

	for _, bad := range [][]byte{{0x3f, 0x01, 0xff}, {0xbf, 0x00}, {0x7f, 0x00}, {0x40, 0x00}} {
		if v, _, err := DecodeWire(bad); errors.Is(err, ErrWire) == false {
			t.Errorf("DecodeWire(% x) = %#v, %v, want ErrWire", bad, v, err)
		}
	}
}
