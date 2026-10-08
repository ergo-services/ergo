package decimal

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

var (
	limitPos = new(big.Int).Lsh(big.NewInt(1), 121)
	limitNeg = new(big.Int).Neg(limitPos)
)

func pow10big(k int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)
}

func refFits(c *big.Int) bool {
	return c.Cmp(limitNeg) >= 0 && c.Cmp(limitPos) < 0
}

func refRound(x *big.Rat, mode RoundingMode) *big.Int {
	num, den := x.Num(), x.Denom()
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	neg := x.Sign() < 0
	r2 := new(big.Int).Abs(r)
	r2.Lsh(r2, 1)
	half := r2.Cmp(den)
	odd := q.Bit(0) == 1
	up := false
	switch mode {
	case RoundHalfUp:
		up = half >= 0
	case RoundHalfEven:
		up = half > 0 || (half == 0 && odd)
	case RoundHalfDown:
		up = half > 0
	case RoundUp:
		up = true
	case RoundDown:
	case RoundCeiling:
		up = neg == false
	case RoundFloor:
		up = neg
	}
	if up {
		if neg {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

func ratScaled(x *big.Rat, k int) *big.Rat {
	if k >= 0 {
		return new(big.Rat).Quo(x, new(big.Rat).SetInt(pow10big(k)))
	}
	return new(big.Rat).Mul(x, new(big.Rat).SetInt(pow10big(-k)))
}

func refFinish(x *big.Rat, scale int, mode RoundingMode) (*big.Int, int, bool) {
	y := new(big.Int).Quo(new(big.Int).Abs(x.Num()), x.Denom())
	yy := new(big.Int).Set(y)
	if x.Sign() < 0 {
		yy.Neg(yy)
	}
	k := 0
	if scale-MaxScale > k {
		k = scale - MaxScale
	}
	if refFits(yy) == false {
		if kd := len(y.String()) - MaxPrecision; kd > k {
			k = kd
		}
	}
	q := refRound(ratScaled(x, k), mode)
	if refFits(q) == false {
		k++
		q = refRound(ratScaled(x, k), mode)
	}
	scale -= k
	if scale < 0 {
		q.Mul(q, pow10big(-scale))
		if refFits(q) == false {
			return nil, 0, true
		}
		scale = 0
	}
	return q, scale, false
}

func coef(d Decimal) *big.Int {
	c := new(big.Int).SetInt64(int64(d.hi) >> scaleBits)
	c.Lsh(c, 64)
	c.Add(c, new(big.Int).SetUint64(d.lo))
	return c
}

func ratOf(d Decimal) *big.Rat {
	return new(big.Rat).SetFrac(coef(d), pow10big(d.scale()))
}

func fromBig(c *big.Int, scale int) Decimal {
	a := new(big.Int).Abs(c)
	lo := new(big.Int).And(a, new(big.Int).SetUint64(^uint64(0))).Uint64()
	hi := new(big.Int).Rsh(a, 64).Uint64()
	return pack(c.Sign() < 0, u128{hi: hi, lo: lo}, scale)
}

func randCoef(r *rand.Rand) *big.Int {
	var c *big.Int
	switch r.Intn(12) {
	case 0:
		c = big.NewInt(int64(r.Intn(3)))
	case 1:
		c = new(big.Int).Sub(limitPos, big.NewInt(int64(r.Intn(3))+1))
	case 2:
		c = new(big.Int).Sub(pow10big(1+r.Intn(37)), big.NewInt(int64(r.Intn(2))))
	case 3:
		c = new(big.Int).Lsh(big.NewInt(1), uint(r.Intn(121)))
		c.Add(c, big.NewInt(int64(r.Intn(3))-1))
	default:
		nd := 1 + r.Intn(37)
		if r.Intn(2) == 0 {
			nd = 1 + r.Intn(19)
		}
		var sb strings.Builder
		for i := 0; i < nd; i++ {
			sb.WriteByte(byte('0' + r.Intn(10)))
		}
		c, _ = new(big.Int).SetString(sb.String(), 10)
	}
	if r.Intn(2) == 0 {
		c.Neg(c)
	}
	if refFits(c) == false {
		c.Quo(c, big.NewInt(10))
	}
	return c
}

func randDecimal(r *rand.Rand) Decimal {
	s := r.Intn(MaxScale + 1)
	if r.Intn(2) == 0 {
		s = r.Intn(19)
	}
	if r.Intn(20) == 0 {
		return fromBig(limitNeg, s)
	}
	return fromBig(randCoef(r), s)
}

func check(t *testing.T, what string, got Decimal, err error, c *big.Int, s int, ovf bool) {
	t.Helper()
	if ovf {
		if err != ErrOverflow {
			t.Fatalf("%s: want overflow, got %v (%s) err %v", what, got.StringScaled(), coef(got), err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: unexpected error %v, want %s scale %d", what, err, c, s)
	}
	if got.scale() != s || coef(got).Cmp(c) != 0 {
		t.Fatalf("%s: got %s scale %d, want %s scale %d", what, coef(got), got.scale(), c, s)
	}
}

var iterations = func() int {
	if n, err := strconv.Atoi(os.Getenv("DECIMAL_ITER")); err == nil {
		return n
	}
	return 300000
}()

func TestAddSubOracle(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < iterations; i++ {
		a, b := randDecimal(r), randDecimal(r)
		s := max(a.scale(), b.scale())
		for _, sub := range []bool{false, true} {
			x := new(big.Rat)
			if sub {
				x.Sub(ratOf(a), ratOf(b))
			} else {
				x.Add(ratOf(a), ratOf(b))
			}
			x.Mul(x, new(big.Rat).SetInt(pow10big(s)))
			c, cs, ovf := refFinish(x, s, RoundHalfUp)
			got, err := a.addSlow(b, sub)
			check(t, a.StringScaled()+" +- "+b.StringScaled(), got, err, c, cs, ovf)
			if ovf == false {
				var fast Decimal
				if sub {
					fast = a.Sub(b)
				} else {
					fast = a.Add(b)
				}
				if fast != got {
					t.Fatalf("fast path differs: %v %v", fast, got)
				}
			}
		}
	}
}

func TestMulOracle(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < iterations; i++ {
		a, b := randDecimal(r), randDecimal(r)
		s := a.scale() + b.scale()
		x := new(big.Rat).Mul(ratOf(a), ratOf(b))
		x.Mul(x, new(big.Rat).SetInt(pow10big(s)))
		c, cs, ovf := refFinish(x, s, RoundHalfUp)
		got, err := a.mulSlow(b)
		check(t, a.StringScaled()+" * "+b.StringScaled(), got, err, c, cs, ovf)
		if ovf == false {
			if fast := a.Mul(b); fast != got {
				t.Fatalf("fast path differs: %v %v", fast, got)
			}
		}
	}
}

func TestDivOracle(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < iterations; i++ {
		a, b := randDecimal(r), randDecimal(r)
		if b.IsZero() {
			if _, err := a.quo(b, 16, RoundHalfUp); err != ErrDivisionByZero {
				t.Fatal("division by zero not detected")
			}
			continue
		}
		places := DivisionPrecision
		switch r.Intn(4) {
		case 0:
			places = r.Intn(50) - 8
		case 1:
			places = r.Intn(5)
		}
		mode := RoundingMode(r.Intn(7))
		p := min(places, MaxScale)
		x := new(big.Rat).Quo(ratOf(a), ratOf(b))
		x = ratScaled(x, -p)
		c, cs, ovf := refFinish(x, p, mode)
		got, err := a.quo(b, places, mode)
		if a.IsZero() {
			c, cs, ovf = big.NewInt(0), max(p, 0), false
		}
		check(t, a.StringScaled()+" / "+b.StringScaled(), got, err, c, cs, ovf)
	}
}

func TestRoundOracle(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < iterations; i++ {
		a := randDecimal(r)
		places := r.Intn(50) - 12
		mode := RoundingMode(r.Intn(7))
		if places >= a.scale() {
			if a.RoundMode(int32(places), mode) != a {
				t.Fatal("round to a larger scale changed the value")
			}
			continue
		}
		x := ratScaled(ratOf(a), -places)
		c, cs, ovf := refFinish(x, places, mode)
		var got Decimal
		var err error
		func() {
			defer func() {
				if v := recover(); v != nil {
					err = v.(error)
				}
			}()
			got = a.RoundMode(int32(places), mode)
		}()
		check(t, a.StringScaled()+" round", got, err, c, cs, ovf)
	}
}

func TestCmpOracle(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for i := 0; i < iterations; i++ {
		a, b := randDecimal(r), randDecimal(r)
		if r.Intn(4) == 0 {
			if c, err := a.Rescale(int32(min(a.scale()+r.Intn(5), MaxScale))); err == nil {
				b = c
			}
		}
		want := ratOf(a).Cmp(ratOf(b))
		if got := a.Cmp(b); got != want {
			t.Fatalf("cmp %s %s: got %d want %d", a.StringScaled(), b.StringScaled(), got, want)
		}
		if got := a.Equal(b); got != (want == 0) {
			t.Fatalf("equal %s %s: got %v", a.StringScaled(), b.StringScaled(), got)
		}
		if a.Sign() != ratOf(a).Sign() {
			t.Fatal("sign")
		}
	}
}

func refString(c *big.Int, s int, trim bool) string {
	neg := c.Sign() < 0
	digits := new(big.Int).Abs(c).String()
	for len(digits) <= s {
		digits = "0" + digits
	}
	ip, fp := digits[:len(digits)-s], digits[len(digits)-s:]
	if trim {
		fp = strings.TrimRight(fp, "0")
	}
	out := ip
	if fp != "" {
		out += "." + fp
	}
	if neg {
		out = "-" + out
	}
	return out
}

func TestStringOracle(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	for i := 0; i < iterations; i++ {
		a := randDecimal(r)
		if got, want := a.String(), refString(coef(a), a.scale(), true); got != want {
			t.Fatalf("String: got %q want %q", got, want)
		}
		if got, want := a.StringScaled(), refString(coef(a), a.scale(), false); got != want {
			t.Fatalf("StringScaled: got %q want %q", got, want)
		}
		b, err := NewFromString(a.StringScaled())
		if err != nil || b != a {
			t.Fatalf("round trip %q: %v %v", a.StringScaled(), b.StringScaled(), err)
		}
		places := r.Intn(45) - 5
		x := ratScaled(ratOf(a), -places)
		if places < a.scale() {
			c, cs, ovf := refFinish(x, places, RoundHalfUp)
			if ovf {
				continue
			}
			want := refString(c, cs, false)
			if places > 0 && cs < places {
				want += strings.Repeat("0", places-cs)
			}
			if got := a.StringFixed(int32(places)); got != want {
				t.Fatalf("StringFixed(%d) of %s: got %q want %q", places, a.StringScaled(), got, want)
			}
		}
	}
}

func randNumberString(r *rand.Rand) string {
	var sb strings.Builder
	switch r.Intn(3) {
	case 0:
		sb.WriteByte('-')
	case 1:
		if r.Intn(4) == 0 {
			sb.WriteByte('+')
		}
	}
	lead := 0
	if r.Intn(4) == 0 {
		lead = r.Intn(30)
	}
	ni := r.Intn(25)
	if r.Intn(10) == 0 {
		ni = r.Intn(90)
	}
	for i := 0; i < lead; i++ {
		sb.WriteByte('0')
	}
	for i := 0; i < ni; i++ {
		sb.WriteByte(byte('0' + r.Intn(10)))
	}
	nf := -1
	if r.Intn(3) > 0 {
		nf = r.Intn(25)
		if r.Intn(10) == 0 {
			nf = r.Intn(90)
		}
		sb.WriteByte('.')
		zeros := 0
		if r.Intn(4) == 0 {
			zeros = r.Intn(40)
		}
		for i := 0; i < zeros; i++ {
			sb.WriteByte('0')
		}
		for i := 0; i < nf; i++ {
			sb.WriteByte(byte('0' + r.Intn(10)))
		}
		nf += zeros
	}
	if ni+lead+max(nf, 0) == 0 {
		sb.WriteByte('7')
	}
	if r.Intn(6) == 0 {
		sb.WriteByte("eE"[r.Intn(2)])
		switch r.Intn(3) {
		case 0:
			sb.WriteByte('-')
		case 1:
			sb.WriteByte('+')
		}
		sb.WriteString(big.NewInt(int64(r.Intn(60))).String())
	}
	return sb.String()
}

func refParse(s string) (*big.Int, int, bool, bool) {
	x, ok := new(big.Rat).SetString(s)
	if ok == false {
		return nil, 0, false, false
	}
	body, exp := s, 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		body = s[:i]
		e, _ := new(big.Int).SetString(strings.TrimPrefix(s[i+1:], "+"), 10)
		exp = int(e.Int64())
	}
	scale := 0
	if i := strings.IndexByte(body, '.'); i >= 0 {
		scale = len(body) - i - 1
	}
	scale -= exp
	c, cs, ovf := refFinish(ratScaled(x, -scale), scale, RoundHalfUp)
	return c, cs, ovf, true
}

func TestParseOracle(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for i := 0; i < iterations; i++ {
		s := randNumberString(r)
		c, cs, ovf, ok := refParse(s)
		got, err := NewFromString(s)
		if ok == false {
			if err == nil {
				t.Fatalf("parse %q: want error, got %s", s, got.StringScaled())
			}
			continue
		}
		check(t, "parse "+s, got, err, c, cs, ovf)
		if b, err2 := NewFromBytes([]byte(s)); err2 != err || b != got {
			t.Fatalf("NewFromBytes differs %q: %#v %#v %v", s, b, got, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{"", "-", "+", ".", "-.", "e5", "1e", "1e+", "1.2.3", "1a", " 1", "1 ", "--1", "1e5.5", "0x10", "1_000", "∞"} {
		if d, err := NewFromString(s); err == nil {
			t.Errorf("%q: want error, got %s", s, d)
		}
	}
	for s, want := range map[string]string{
		"1.": "1", ".5": "0.5", "-.5": "-0.5", "+1": "1", "-0": "0", "00012.3400": "12.34",
		"1e3": "1000", "1.5E-3": "0.0015", "-0.000": "0",
		"123456789012345678901234567890.123456":   "123456789012345678901234567890.123456",
		"0.1234567890123456789012345678901234567": "0.123456789012345678901234567890123457",
		"2658455991569831745807614120560689151":   "2658455991569831745807614120560689151",
		"2658455991569831745807614120560689152":   "2658455991569831745807614120560689150",
	} {
		d, err := NewFromString(s)
		if err != nil || d.String() != want {
			t.Errorf("%q: got %q %v want %q", s, d.String(), err, want)
		}
	}
	if _, err := NewFromString("26584559915698317458076141205606891520"); err != ErrOverflow {
		t.Error("overflow not detected")
	}
}

func TestWire(t *testing.T) {
	r := rand.New(rand.NewSource(8))
	for i := 0; i < iterations; i++ {
		a := randDecimal(r)
		b, _ := a.MarshalBinary()
		if len(b) != a.WireLen() {
			t.Fatal("wire length")
		}
		var c Decimal
		if err := c.UnmarshalBinary(b); err != nil || c != a {
			t.Fatalf("wire round trip %s: %v %v", a.StringScaled(), c.StringScaled(), err)
		}
		app, _ := a.AppendBinary([]byte{1, 2})
		if string(app[2:]) != string(b) {
			t.Fatal("append binary")
		}
	}
	for _, b := range [][]byte{nil, {0}, {37, 0}, {0x40, 0}, {0, 17}, {0, 2, 1}, {0, 16, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}} {
		var d Decimal
		if err := d.UnmarshalBinary(b); err == nil {
			t.Errorf("%v: want error", b)
		}
	}
	min := []byte{0x80, 16, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	var d Decimal
	if err := d.UnmarshalBinary(min); err != nil || coef(d).Cmp(limitNeg) != 0 {
		t.Error("-2^121")
	}
}

func TestJSON(t *testing.T) {
	d := RequireFromString("-1234.5600")
	b, _ := d.MarshalJSON()
	if string(b) != `"-1234.56"` {
		t.Fatal(string(b))
	}
	var e Decimal
	for _, s := range []string{`"-1234.56"`, `-1234.56`, `-1234.5600`} {
		if err := e.UnmarshalJSON([]byte(s)); err != nil || e.Equal(d) == false {
			t.Fatal(s, err)
		}
	}
	e = d
	if err := e.UnmarshalJSON([]byte("null")); err != nil || e != d {
		t.Fatal("null")
	}
}

func TestFloat(t *testing.T) {
	for _, f := range []float64{0, 0.1, -0.1, 123.456, 1e-7, 3.14159, 1e20, 2.5e-30, 123456789.123456789} {
		d := NewFromFloat(f)
		if g := d.InexactFloat64(); g != f {
			t.Errorf("%v -> %s -> %v", f, d, g)
		}
	}
	if f, exact := RequireFromString("0.5").Float64(); f != 0.5 || exact == false {
		t.Error("0.5 exact")
	}
	if _, exact := RequireFromString("0.1").Float64(); exact {
		t.Error("0.1 inexact")
	}
	if NewFromFloat(0.1).String() != "0.1" {
		t.Error("0.1")
	}
}

func TestFloatExponentOracle(t *testing.T) {
	r := rand.New(rand.NewSource(8))
	for i := 0; i < iterations; i++ {
		var f float64
		switch r.Intn(3) {
		case 0:
			f = math.Float64frombits(uint64(r.Intn(211)+943)<<52 | r.Uint64()&(1<<52-1))
		case 1:
			f = float64(r.Int63n(1e15)) / math.Pow10(r.Intn(19))
		default:
			f = float64(r.Int63n(1<<40)) / float64(uint64(1)<<r.Intn(40))
		}
		if r.Intn(2) == 0 {
			f = -f
		}
		exp := int32(r.Intn(81) - 40)
		x := new(big.Rat).SetFloat64(f)
		x = ratScaled(x, int(exp))
		c, cs, ovf := refFinish(x, -int(exp), RoundHalfUp)
		got, err := newFromFloatExp(f, exp)
		check(t, fmt.Sprintf("%v e%d", f, exp), got, err, c, cs, ovf)
	}
}

func TestMisc(t *testing.T) {
	if New(15, -1).String() != "1.5" || New(15, 2).String() != "1500" {
		t.Error("New")
	}
	if RequireFromString("-7.5").IntPart() != -7 {
		t.Error("IntPart")
	}
	if RequireFromString("10").Div(RequireFromString("3")).String() != "3.3333333333333333" {
		t.Error("Div")
	}
	if RequireFromString("2").Div(RequireFromString("3")).String() != "0.6666666666666667" {
		t.Error("Div round")
	}
	q, rem := RequireFromString("10").QuoRem(RequireFromString("3"), 0)
	if q.String() != "3" || rem.String() != "1" {
		t.Error("QuoRem", q, rem)
	}
	if RequireFromString("-7.5").Mod(RequireFromString("2")).String() != "-1.5" {
		t.Error("Mod")
	}
	if RequireFromString("1.500").Canonical() != RequireFromString("1.5") {
		t.Error("Canonical")
	}
	if RequireFromString("1.50").IsInteger() || RequireFromString("2.000").IsInteger() == false {
		t.Error("IsInteger")
	}
	if RequireFromString("-2.5").Floor().String() != "-3" || RequireFromString("-2.5").Ceil().String() != "-2" {
		t.Error("Floor/Ceil")
	}
	if Avg(RequireFromString("1"), RequireFromString("2")).String() != "1.5" {
		t.Error("Avg")
	}
	if RequireFromString("123.456").Coefficient().String() != "123456" {
		t.Error("Coefficient")
	}
	if NewFromBigInt(big.NewInt(-12345), -2).String() != "-123.45" {
		t.Error("NewFromBigInt")
	}
}
