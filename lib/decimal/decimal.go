// Package decimal implements a fixed-size decimal number for money and other
// financial quantities: amounts, prices, rates, fees.
//
// A Decimal is 16 bytes with no pointers: a 122-bit two's complement
// coefficient and a scale 0..36, value = coefficient * 10^-scale. It holds
// 36 significant digits (37 below 2^121), so 18 integer digits with 18
// fractional ones, or 12 with 24, are always exact. Values are passed by
// value, and arithmetic does not allocate.
//
// Add, Sub and Mul are exact whenever the exact result is representable.
// Otherwise the result is rounded once, half away from zero, to 36
// significant digits; if the integer part itself does not fit, the operation
// overflows (methods panic, Checked* methods return ErrOverflow). Div rounds
// to DivisionPrecision places, half away from zero; DivRound takes the
// places, QuoRound the places and the RoundingMode.
//
// The zero value is 0. Values are compared with Cmp/Equal: 1.5 and 1.50 are
// equal numbers with different scales, so == on Decimal compares
// representations, not values. Canonical gives one representation per
// number.
//
// The method set follows github.com/shopspring/decimal, so most code moves
// over by changing the import path. Text, JSON, database/sql and EDF
// encodings are built in; the binary form takes 2 to 18 bytes.
package decimal

import (
	"errors"
	"math/bits"
)

const (
	// MaxScale is the largest number of digits after the decimal point.
	MaxScale = 36
	// MaxPrecision is the number of significant digits always representable.
	MaxPrecision = 36
	// DivisionPrecision is the number of places Div rounds the quotient to.
	DivisionPrecision = 16

	scaleBits = 6
	scaleMask = 1<<scaleBits - 1
	limitHi   = 1 << 57
)

var (
	ErrOverflow       = errors.New("decimal: overflow")
	ErrDivisionByZero = errors.New("decimal: division by zero")
	ErrSyntax         = errors.New("decimal: invalid syntax")
)

// Decimal is a fixed-size decimal number. See the package documentation.
type Decimal struct {
	hi uint64
	lo uint64
}

// RoundingMode selects how a dropped fraction is rounded.
type RoundingMode uint8

const (
	RoundHalfUp   RoundingMode = iota // half away from zero (commercial)
	RoundHalfEven                     // half to even (banker's)
	RoundHalfDown                     // half toward zero
	RoundUp                           // away from zero
	RoundDown                         // toward zero (truncate)
	RoundCeiling                      // toward +infinity
	RoundFloor                        // toward -infinity
)

var (
	Zero = Decimal{}
	One  = Decimal{lo: 1}
)

func (d Decimal) scale() int {
	return int(d.hi & scaleMask)
}

func (d Decimal) parts() (neg bool, m u128, scale int) {
	h := uint64(int64(d.hi) >> scaleBits)
	l := d.lo
	if int64(h) < 0 {
		var b uint64
		l, b = bits.Sub64(0, l, 0)
		h, _ = bits.Sub64(0, h, b)
		neg = true
	}
	return neg, u128{hi: h, lo: l}, int(d.hi & scaleMask)
}

func fits(neg bool, m u128) bool {
	return m.hi < limitHi || (neg && m.hi == limitHi && m.lo == 0)
}

func fits256(neg bool, m u256) bool {
	return m.w3|m.w2 == 0 && fits(neg, m.lo128())
}

func pack(neg bool, m u128, scale int) Decimal {
	h, l := m.hi, m.lo
	if neg {
		var b uint64
		l, b = bits.Sub64(0, l, 0)
		h, _ = bits.Sub64(0, h, b)
	}
	return Decimal{hi: h<<scaleBits | uint64(scale), lo: l}
}

func roundUp(mode RoundingMode, neg, odd bool, half int, nz bool) bool {
	switch mode {
	case RoundHalfUp:
		return half >= 0
	case RoundHalfEven:
		return half > 0 || (half == 0 && odd)
	case RoundHalfDown:
		return half > 0
	case RoundUp:
		return nz
	case RoundCeiling:
		return nz && neg == false
	case RoundFloor:
		return nz && neg
	}
	return false
}

func cmpHalf(r, d u128) int {
	if r.hi>>63 != 0 {
		return 1
	}
	return cmp128(u128{hi: r.hi<<1 | r.lo>>63, lo: r.lo << 1}, d)
}

func shrink(q u256, k int, nz bool) (u256, int, bool) {
	if k >= len(pow10tab256) {
		return u256{}, -1, nz || q.isZero() == false
	}
	var r uint64
	for k > 19 {
		q, r = divPow10x256(q, 19)
		nz = nz || r != 0
		k -= 19
	}
	q, r = divPow10x256(q, k)
	h := cmpHalf64(r, pow10tab64[k])
	if h == 0 && nz {
		h = 1
	}
	return q, h, nz || r != 0
}

func cmpHalf64(r, d uint64) int {
	if r>>63 != 0 {
		return 1
	}
	r <<= 1
	if r < d {
		return -1
	}
	if r > d {
		return 1
	}
	return 0
}

func finish(neg bool, q u256, scale int, half int, nz bool, mode RoundingMode) (Decimal, error) {
	k := scale - MaxScale
	if fits256(neg, q) == false {
		if kd := digits256(q) - MaxPrecision; kd > k {
			k = kd
		}
	}
	if k > 0 {
		q, half, nz = shrink(q, k, nz)
		scale -= k
	}
	if roundUp(mode, neg, q.w0&1 == 1, half, nz) {
		q1 := inc256(q)
		if fits256(neg, q1) {
			q = q1
		} else {
			q, half, nz = shrink(q, 1, nz)
			scale--
			if roundUp(mode, neg, q.w0&1 == 1, half, nz) {
				q = inc256(q)
			}
		}
	}
	if scale < 0 {
		if q.isZero() {
			return Decimal{}, nil
		}
		if -scale > MaxPrecision+1 {
			return Decimal{}, ErrOverflow
		}
		q = mulPow10(q.lo128(), -scale)
		if fits256(neg, q) == false {
			return Decimal{}, ErrOverflow
		}
		scale = 0
	}
	return pack(neg, q.lo128(), scale), nil
}

func must(d Decimal, err error) Decimal {
	if err != nil {
		panic(err)
	}
	return d
}

// New returns value * 10^exp.
func New(value int64, exp int32) Decimal {
	d := NewFromInt(value)
	if exp >= 0 {
		return must(d.shift(int(exp)))
	}
	neg, m, _ := d.parts()
	return must(finish(neg, to256(m), int(-exp), -1, false, RoundHalfUp))
}

// NewFromInt returns the integer v.
func NewFromInt(v int64) Decimal {
	return Decimal{hi: uint64(v>>63) << scaleBits, lo: uint64(v)}
}

// NewFromInt32 returns the integer v.
func NewFromInt32(v int32) Decimal {
	return NewFromInt(int64(v))
}

// NewFromUint64 returns the integer v.
func NewFromUint64(v uint64) Decimal {
	return Decimal{lo: v}
}

// NewFromParts returns coef * 10^-scale, rounding half away from zero if
// the scale exceeds MaxScale.
func NewFromParts(coef int64, scale int) Decimal {
	return New(coef, int32(-scale))
}

// Scale returns the number of digits after the decimal point.
func (d Decimal) Scale() int {
	return d.scale()
}

// Exponent returns the exponent: d = coefficient * 10^Exponent.
func (d Decimal) Exponent() int32 {
	return -int32(d.scale())
}

// Sign returns -1, 0 or 1.
func (d Decimal) Sign() int {
	h := int64(d.hi) >> scaleBits
	if h < 0 {
		return -1
	}
	if h == 0 && d.lo == 0 {
		return 0
	}
	return 1
}

// IsZero reports whether d == 0.
func (d Decimal) IsZero() bool {
	return d.hi>>scaleBits == 0 && d.lo == 0
}

// IsNegative reports whether d < 0.
func (d Decimal) IsNegative() bool {
	return int64(d.hi) < 0
}

// IsPositive reports whether d > 0.
func (d Decimal) IsPositive() bool {
	return d.Sign() > 0
}

// IsInteger reports whether d has no fractional part.
func (d Decimal) IsInteger() bool {
	s := d.scale()
	if s == 0 {
		return true
	}
	_, m, _ := d.parts()
	_, _, nz := shrink(to256(m), s, false)
	return nz == false
}

// CoefficientInt64 returns the low 64 bits of the coefficient.
func (d Decimal) CoefficientInt64() int64 {
	return int64(d.lo)
}

// NumDigits returns the number of digits of the coefficient.
func (d Decimal) NumDigits() int {
	_, m, _ := d.parts()
	if n := digits128(m); n > 0 {
		return n
	}
	return 1
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	l, b := bits.Sub64(0, d.lo, 0)
	h := -(int64(d.hi) >> scaleBits) - int64(b)
	if h<<scaleBits>>scaleBits != h {
		panic(ErrOverflow)
	}
	return Decimal{hi: uint64(h)<<scaleBits | d.hi&scaleMask, lo: l}
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	if int64(d.hi) < 0 {
		return d.Neg()
	}
	return d
}

// Add returns d + e.
func (d Decimal) Add(e Decimal) Decimal {
	if (d.hi^e.hi)&scaleMask == 0 {
		lo, c := bits.Add64(d.lo, e.lo, 0)
		hi := int64(d.hi)>>scaleBits + int64(e.hi)>>scaleBits + int64(c)
		if hi<<scaleBits>>scaleBits == hi {
			return Decimal{hi: uint64(hi)<<scaleBits | d.hi&scaleMask, lo: lo}
		}
	}
	return d.addAligned(e, false)
}

// Sub returns d - e.
func (d Decimal) Sub(e Decimal) Decimal {
	if (d.hi^e.hi)&scaleMask == 0 {
		lo, b := bits.Sub64(d.lo, e.lo, 0)
		hi := int64(d.hi)>>scaleBits - int64(e.hi)>>scaleBits - int64(b)
		if hi<<scaleBits>>scaleBits == hi {
			return Decimal{hi: uint64(hi)<<scaleBits | d.hi&scaleMask, lo: lo}
		}
	}
	return d.addAligned(e, true)
}

func (d Decimal) addAligned(e Decimal, sub bool) Decimal {
	xh, xl := int64(d.hi)>>scaleBits, d.lo
	yh, yl := int64(e.hi)>>scaleBits, e.lo
	sd, se := d.hi&scaleMask, e.hi&scaleMask
	s := sd
	ok := true
	switch {
	case sd < se:
		xh, xl, ok = scaleUp(d, se-sd)
		s = se
	case sd > se:
		yh, yl, ok = scaleUp(e, sd-se)
	}
	if ok {
		var lo, c uint64
		var hi int64
		if sub {
			lo, c = bits.Sub64(xl, yl, 0)
			hi = xh - yh - int64(c)
		} else {
			lo, c = bits.Add64(xl, yl, 0)
			hi = xh + yh + int64(c)
		}
		if hi<<scaleBits>>scaleBits == hi {
			return Decimal{hi: uint64(hi)<<scaleBits | s, lo: lo}
		}
	}
	return must(d.addSlow(e, sub))
}

func aligned(d, e Decimal) (xh int64, xl uint64, yh int64, yl uint64, s uint64, ok bool) {
	sd, se := d.hi&scaleMask, e.hi&scaleMask
	if sd < se {
		xh, xl, ok = scaleUp(d, se-sd)
		return xh, xl, int64(e.hi) >> scaleBits, e.lo, se, ok
	}
	yh, yl, ok = scaleUp(e, sd-se)
	return int64(d.hi) >> scaleBits, d.lo, yh, yl, sd, ok
}

func scaleUp(d Decimal, k uint64) (int64, uint64, bool) {
	x := int64(d.lo)
	if int64(d.hi)>>scaleBits != x>>63 || k > 18 {
		return 0, 0, false
	}
	p := pow10tab64[k]
	hi, lo := bits.Mul64(uint64(x), p)
	hi -= uint64(x>>63) & p
	return int64(hi), lo, int64(hi)<<scaleBits>>scaleBits == int64(hi)
}

// CheckedAdd returns d + e or ErrOverflow.
func (d Decimal) CheckedAdd(e Decimal) (Decimal, error) {
	return d.addSlow(e, false)
}

// CheckedSub returns d - e or ErrOverflow.
func (d Decimal) CheckedSub(e Decimal) (Decimal, error) {
	return d.addSlow(e, true)
}

func (d Decimal) addSlow(e Decimal, sub bool) (Decimal, error) {
	nd, md, sd := d.parts()
	ne, me, se := e.parts()
	if sub {
		ne = ne == false
	}
	a, b := to256(md), to256(me)
	scale := sd
	switch {
	case sd < se:
		a = mulPow10(md, se-sd)
		scale = se
	case sd > se:
		b = mulPow10(me, sd-se)
	}
	var r u256
	neg := nd
	switch {
	case nd == ne:
		r = add256(a, b)
	case cmp256(a, b) >= 0:
		r = sub256(a, b)
	default:
		r = sub256(b, a)
		neg = ne
	}
	return finish(neg, r, scale, -1, false, RoundHalfUp)
}

// Mul returns d * e.
func (d Decimal) Mul(e Decimal) Decimal {
	if int64(d.hi)>>scaleBits == int64(d.lo)>>63 && int64(e.hi)>>scaleBits == int64(e.lo)>>63 {
		x, y := int64(d.lo), int64(e.lo)
		hi, lo := bits.Mul64(uint64(x), uint64(y))
		hi -= uint64(x>>63)&uint64(y) + uint64(y>>63)&uint64(x)
		s := d.hi&scaleMask + e.hi&scaleMask
		if int64(hi)<<scaleBits>>scaleBits == int64(hi) && s <= MaxScale {
			return Decimal{hi: hi<<scaleBits | s, lo: lo}
		}
	}
	return must(d.mulSlow(e))
}

// CheckedMul returns d * e or ErrOverflow.
func (d Decimal) CheckedMul(e Decimal) (Decimal, error) {
	return d.mulSlow(e)
}

func (d Decimal) mulSlow(e Decimal) (Decimal, error) {
	nd, md, sd := d.parts()
	ne, me, se := e.parts()
	neg := nd != ne
	p := mul128(md, me)
	s := sd + se
	k := s - MaxScale
	if fits256(neg, p) == false {
		if kd := digits256(p) - MaxPrecision; kd > k {
			k = kd
		}
	}
	if k <= 0 {
		return pack(neg, p.lo128(), s), nil
	}
	if k < len(pow10tab64) && k <= s {
		q, r := divPow10x256(p, k)
		m := q.lo128()
		if cmpHalf64(r, pow10tab64[k]) >= 0 {
			m = inc128(m)
		}
		if fits(neg, m) {
			return pack(neg, m, s-k), nil
		}
	}
	return finish(neg, p, s, -1, false, RoundHalfUp)
}

// Div returns d / e rounded half away from zero to DivisionPrecision places.
// It panics if e is zero.
func (d Decimal) Div(e Decimal) Decimal {
	return must(d.quo(e, DivisionPrecision, RoundHalfUp))
}

// DivRound returns d / e rounded half away from zero to places digits after
// the point (to 36 significant digits if that is less). Places beyond
// MaxScale are MaxScale: DivRound(e, MaxScale) is the most precise quotient.
func (d Decimal) DivRound(e Decimal, places int32) Decimal {
	return must(d.quo(e, int(places), RoundHalfUp))
}

// QuoRound returns d / e rounded with the mode to places digits.
func (d Decimal) QuoRound(e Decimal, places int32, mode RoundingMode) (Decimal, error) {
	return d.quo(e, int(places), mode)
}

// CheckedDiv is Div returning ErrDivisionByZero or ErrOverflow.
func (d Decimal) CheckedDiv(e Decimal) (Decimal, error) {
	return d.quo(e, DivisionPrecision, RoundHalfUp)
}

func (d Decimal) quo(e Decimal, places int, mode RoundingMode) (Decimal, error) {
	ne, b, sb := e.parts()
	if b.isZero() {
		return Decimal{}, ErrDivisionByZero
	}
	na, a, sa := d.parts()
	if places > MaxScale {
		places = MaxScale
	}
	if a.isZero() {
		return Decimal{hi: uint64(max(places, 0))}, nil
	}
	neg := na != ne

	k := places + sb - sa
	if a.hi|b.hi == 0 && uint(k) < uint(len(pow10tab64)) && places >= 0 {
		nh, nl := bits.Mul64(a.lo, pow10tab64[k])
		var q u128
		var r uint64
		switch {
		case nh == 0:
			q.lo, r = nl/b.lo, nl%b.lo
		case nh < b.lo:
			q.lo, r = bits.Div64(nh, nl, b.lo)
		default:
			q.hi, r = nh/b.lo, nh%b.lo
			q.lo, r = bits.Div64(r, nl, b.lo)
		}
		if roundUp(mode, neg, q.lo&1 == 1, cmpHalf64(r, b.lo), r != 0) {
			q = inc128(q)
		}
		if fits(neg, q) {
			return pack(neg, q, places), nil
		}
	}

	if k >= 0 && k < len(pow10tab128) && places >= 0 {
		if n := mulPow10(a, k); n.w3 == 0 {
			q, r := divmod256x128(n, b)
			m := q.lo128()
			if r.isZero() == false && roundUp(mode, neg, m.lo&1 == 1, cmpHalf(r, b), true) {
				m = inc128(m)
			}
			if q.w3|q.w2 == 0 && fits(neg, m) {
				return pack(neg, m, places), nil
			}
		}
	}

	p := places
	if ex := digits128(a) - digits128(b) + k - 38; ex > 0 {
		k -= ex
		p -= ex
	}
	var q u256
	half, nz := -1, true
	switch {
	case k >= 0:
		var r u128
		q, r = divmod256x128(mulPow10(a, k), b)
		half, nz = cmpHalf(r, b), r.isZero() == false
	case -k < len(pow10tab128):
		dv := mul128(b, pow10tab128[-k])
		if dv.w3|dv.w2 == 0 {
			var r u128
			dd := dv.lo128()
			q, r = divmod256x128(to256(a), dd)
			half, nz = cmpHalf(r, dd), r.isZero() == false
		}
	}
	return finish(neg, q, p, half, nz, mode)
}

// QuoRem returns the quotient q, truncated to places digits, and the
// remainder r = d - e*q.
func (d Decimal) QuoRem(e Decimal, places int32) (Decimal, Decimal) {
	q := must(d.quo(e, int(places), RoundDown))
	return q, d.Sub(e.Mul(q))
}

// Mod returns d modulo e (the remainder of the truncated integer quotient).
func (d Decimal) Mod(e Decimal) Decimal {
	_, r := d.QuoRem(e, 0)
	return r
}

// Shift returns d * 10^n.
func (d Decimal) Shift(n int32) Decimal {
	return must(d.shift(int(n)))
}

func (d Decimal) shift(n int) (Decimal, error) {
	neg, m, s := d.parts()
	if n <= s {
		return finish(neg, to256(m), s-n, -1, false, RoundHalfUp)
	}
	n -= s
	if m.isZero() {
		return Decimal{}, nil
	}
	if n > MaxPrecision+1 {
		return Decimal{}, ErrOverflow
	}
	r := mulPow10(m, n)
	if fits256(neg, r) == false {
		return Decimal{}, ErrOverflow
	}
	return pack(neg, r.lo128(), 0), nil
}

// RoundMode rounds d to places digits after the point with the mode. A
// negative places rounds the integer part. A places not less than the scale
// returns d unchanged.
func (d Decimal) RoundMode(places int32, mode RoundingMode) Decimal {
	k := int(d.hi&scaleMask) - int(places)
	if k <= 0 {
		return d
	}
	if k < len(pow10tab64) && places >= 0 {
		neg, m, _ := d.parts()
		p := pow10tab64[k]
		var q u128
		var r uint64
		if m.hi == 0 {
			q.lo, r = m.lo/p, m.lo%p
		} else {
			q, r = divPow10x128(m, k)
		}
		if r != 0 && roundUp(mode, neg, q.lo&1 == 1, cmpHalf64(r, p), true) {
			q = inc128(q)
		}
		if fits(neg, q) {
			return pack(neg, q, int(places))
		}
	}
	return d.roundSlow(int(places), k, mode)
}

func (d Decimal) roundSlow(places, k int, mode RoundingMode) Decimal {
	neg, m, _ := d.parts()
	q, half, nz := shrink(to256(m), k, false)
	return must(finish(neg, q, places, half, nz, mode))
}

// Round rounds half away from zero to places digits after the point.
func (d Decimal) Round(places int32) Decimal {
	return d.RoundMode(places, RoundHalfUp)
}

// RoundBank rounds half to even to places digits after the point.
func (d Decimal) RoundBank(places int32) Decimal {
	return d.RoundMode(places, RoundHalfEven)
}

// RoundUp rounds away from zero to places digits after the point.
func (d Decimal) RoundUp(places int32) Decimal {
	return d.RoundMode(places, RoundUp)
}

// RoundDown rounds toward zero to places digits after the point.
func (d Decimal) RoundDown(places int32) Decimal {
	return d.RoundMode(places, RoundDown)
}

// RoundCeil rounds toward +infinity to places digits after the point.
func (d Decimal) RoundCeil(places int32) Decimal {
	return d.RoundMode(places, RoundCeiling)
}

// RoundFloor rounds toward -infinity to places digits after the point.
func (d Decimal) RoundFloor(places int32) Decimal {
	return d.RoundMode(places, RoundFloor)
}

// Truncate drops the digits after places digits after the point.
func (d Decimal) Truncate(places int32) Decimal {
	return d.RoundMode(places, RoundDown)
}

// Floor returns the nearest integer not greater than d.
func (d Decimal) Floor() Decimal {
	return d.RoundMode(0, RoundFloor)
}

// Ceil returns the nearest integer not less than d.
func (d Decimal) Ceil() Decimal {
	return d.RoundMode(0, RoundCeiling)
}

// Rescale returns d with exactly the scale digits after the point: rounding
// half away from zero when shrinking, padding with zeros when growing.
func (d Decimal) Rescale(scale int32) (Decimal, error) {
	s := d.scale()
	if scale < 0 || scale > MaxScale {
		return Decimal{}, ErrOverflow
	}
	if int(scale) <= s {
		return d.RoundMode(scale, RoundHalfUp), nil
	}
	neg, m, _ := d.parts()
	r := mulPow10(m, int(scale)-s)
	if fits256(neg, r) == false {
		return Decimal{}, ErrOverflow
	}
	return pack(neg, r.lo128(), int(scale)), nil
}

// Canonical returns d with the trailing zeros of the fraction removed: equal
// values have equal canonical representations (usable as map keys).
func (d Decimal) Canonical() Decimal {
	s := d.scale()
	if s == 0 {
		return d
	}
	neg, m, _ := d.parts()
	if m.isZero() {
		return Decimal{}
	}
	for s > 0 {
		q, r := divPow10x128(m, 1)
		if r != 0 {
			break
		}
		m = q
		s--
	}
	return pack(neg, m, s)
}

// Cmp returns -1, 0 or 1 for d < e, d == e, d > e.
func (d Decimal) Cmp(e Decimal) int {
	if (d.hi^e.hi)&scaleMask == 0 {
		if d.hi != e.hi {
			if int64(d.hi) < int64(e.hi) {
				return -1
			}
			return 1
		}
		if d.lo != e.lo {
			if d.lo < e.lo {
				return -1
			}
			return 1
		}
		return 0
	}
	return d.cmpSlow(e)
}

func (d Decimal) cmpSlow(e Decimal) int {
	if xh, xl, yh, yl, _, ok := aligned(d, e); ok {
		switch {
		case xh < yh:
			return -1
		case xh > yh:
			return 1
		case xl < yl:
			return -1
		case xl > yl:
			return 1
		}
		return 0
	}
	sd, se := d.Sign(), e.Sign()
	if sd != se {
		if sd < se {
			return -1
		}
		return 1
	}
	if sd == 0 {
		return 0
	}
	_, md, xd := d.parts()
	_, me, xe := e.parts()
	var c int
	if xd < xe {
		c = cmp256(mulPow10(md, xe-xd), to256(me))
	} else {
		c = cmp256(to256(md), mulPow10(me, xd-xe))
	}
	if sd < 0 {
		return -c
	}
	return c
}

// Compare is Cmp.
func (d Decimal) Compare(e Decimal) int {
	return d.Cmp(e)
}

// Equal reports whether d and e are equal numbers.
func (d Decimal) Equal(e Decimal) bool {
	if d == e {
		return true
	}
	if (d.hi^e.hi)&scaleMask == 0 {
		return false
	}
	return d.cmpSlow(e) == 0
}

// Equals is Equal.
func (d Decimal) Equals(e Decimal) bool {
	return d.Equal(e)
}

// GreaterThan reports whether d > e.
func (d Decimal) GreaterThan(e Decimal) bool {
	return d.Cmp(e) > 0
}

// GreaterThanOrEqual reports whether d >= e.
func (d Decimal) GreaterThanOrEqual(e Decimal) bool {
	return d.Cmp(e) >= 0
}

// LessThan reports whether d < e.
func (d Decimal) LessThan(e Decimal) bool {
	return d.Cmp(e) < 0
}

// LessThanOrEqual reports whether d <= e.
func (d Decimal) LessThanOrEqual(e Decimal) bool {
	return d.Cmp(e) <= 0
}

// Min returns the smallest of the arguments.
func Min(first Decimal, rest ...Decimal) Decimal {
	m := first
	for _, d := range rest {
		if d.Cmp(m) < 0 {
			m = d
		}
	}
	return m
}

// Max returns the largest of the arguments.
func Max(first Decimal, rest ...Decimal) Decimal {
	m := first
	for _, d := range rest {
		if d.Cmp(m) > 0 {
			m = d
		}
	}
	return m
}

// Sum returns the sum of the arguments.
func Sum(first Decimal, rest ...Decimal) Decimal {
	s := first
	for _, d := range rest {
		s = s.Add(d)
	}
	return s
}

// Avg returns the average of the arguments.
func Avg(first Decimal, rest ...Decimal) Decimal {
	return Sum(first, rest...).Div(NewFromInt(int64(len(rest) + 1)))
}
