package decimal

import (
	"math"
	"math/big"
	"math/bits"
	"strconv"
)

var pow10f = [...]float64{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10,
	1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22}

var pow5 = func() (t [23]uint64) {
	t[0] = 1
	for i := 1; i < len(t); i++ {
		t[i] = t[i-1] * 5
	}
	return
}()

// NewFromFloat returns the shortest decimal that converts back to f
// (0.1 for 0.1), rounded to MaxScale places. It panics on NaN and Inf.
func NewFromFloat(f float64) Decimal {
	return newFromFloat(f, 64)
}

// NewFromFloat32 is NewFromFloat for a float32.
func NewFromFloat32(f float32) Decimal {
	return newFromFloat(float64(f), 32)
}

func newFromFloat(f float64, size int) Decimal {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		panic("decimal: cannot convert NaN or Inf")
	}
	var buf [32]byte
	return must(parse(strconv.AppendFloat(buf[:0], f, 'e', -1, size)))
}

// NewFromFloatWithExponent returns the exact binary value of f rounded half
// away from zero to a multiple of 10^exp (at most MaxScale places): 2.675 is
// 2.67499999... in binary and gives 2.67 for exp -2.
func NewFromFloatWithExponent(f float64, exp int32) Decimal {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		panic("decimal: cannot convert NaN or Inf")
	}
	return must(newFromFloatExp(f, exp))
}

func newFromFloatExp(f float64, exp int32) (Decimal, error) {
	b := math.Float64bits(f)
	neg := b>>63 != 0
	mant, e := b&(1<<52-1), int(b>>52&0x7ff)
	if e == 0 {
		e = 1
	} else {
		mant |= 1 << 52
	}
	e -= 1075
	if mant == 0 {
		return Decimal{}, nil
	}
	if bits.Len64(mant)+e > 123 {
		if exp > 308 {
			return Decimal{}, nil
		}
		if exp > 0 {
			twice := new(big.Int).Lsh(new(big.Int).SetUint64(mant), uint(e+1))
			if twice.Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)) < 0 {
				return Decimal{}, nil
			}
		}
		return Decimal{}, ErrOverflow
	}
	places := -int(exp)
	if places > MaxScale+1 {
		places = MaxScale + 1
	}
	q := u256{w0: mant}
	if places > 0 {
		q = mulPow10(u128{lo: mant}, places)
	}
	half, nz := -1, false
	if e >= 0 {
		q = shl256(q, uint(e))
	} else {
		q, half, nz = shr256(q, uint(-e))
	}
	if places < 0 {
		q, half, nz = shrink(q, -places, nz)
	}
	return finish(neg, q, places, half, nz, RoundHalfUp)
}

// Float64 returns the nearest float64 and whether it is exactly d.
func (d Decimal) Float64() (f float64, exact bool) {
	neg, m, s := d.parts()
	if m.hi == 0 && m.lo < 1<<53 && s < len(pow10f) {
		f = float64(m.lo) / pow10f[s]
		if neg {
			f = -f
		}
		return f, m.lo%pow5[s] == 0
	}
	f, _ = d.Rat().Float64()
	return f, d.Rat().Cmp(new(big.Rat).SetFloat64(f)) == 0
}

// InexactFloat64 returns the nearest float64.
func (d Decimal) InexactFloat64() float64 {
	neg, m, s := d.parts()
	if m.hi == 0 && m.lo < 1<<53 && s < len(pow10f) {
		f := float64(m.lo) / pow10f[s]
		if neg {
			f = -f
		}
		return f
	}
	var b fmtBuf
	i, j := b.format(d, false)
	f, _ := strconv.ParseFloat(string(b[i:j]), 64)
	return f
}

// IntPart returns the integer part (truncated toward zero) as an int64; the
// low 64 bits if it does not fit.
func (d Decimal) IntPart() int64 {
	t := d.RoundMode(0, RoundDown)
	return int64(t.lo)
}

// BigInt returns the integer part (truncated toward zero).
func (d Decimal) BigInt() *big.Int {
	return d.RoundMode(0, RoundDown).Coefficient()
}

// Coefficient returns the coefficient: d = Coefficient * 10^Exponent.
func (d Decimal) Coefficient() *big.Int {
	neg, m, _ := d.parts()
	b := new(big.Int).SetUint64(m.hi)
	b.Lsh(b, 64)
	b.Or(b, new(big.Int).SetUint64(m.lo))
	if neg {
		b.Neg(b)
	}
	return b
}

// Rat returns d as a rational number.
func (d Decimal) Rat() *big.Rat {
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d.scale())), nil)
	return new(big.Rat).SetFrac(d.Coefficient(), den)
}

// NewFromBigInt returns v * 10^exp, rounding half away from zero beyond 36
// significant digits. It panics if the integer part does not fit.
func NewFromBigInt(v *big.Int, exp int32) Decimal {
	d, err := newFromBig(v, exp)
	if err != nil {
		panic(err)
	}
	return d
}

func newFromBig(v *big.Int, exp int32) (Decimal, error) {
	if v.BitLen() <= 256 {
		var m u256
		words := new(big.Int).Abs(v).Bits()
		var limbs [4]uint64
		if bits.UintSize == 64 {
			for i, w := range words {
				limbs[i] = uint64(w)
			}
		} else {
			for i, w := range words {
				limbs[i/2] |= uint64(w) << (32 * (i % 2))
			}
		}
		m = u256{limbs[0], limbs[1], limbs[2], limbs[3]}
		return finish(v.Sign() < 0, m, -int(exp), -1, false, RoundHalfUp)
	}
	var buf []byte
	buf = v.Append(buf, 10)
	buf = append(buf, 'e')
	buf = strconv.AppendInt(buf, int64(exp), 10)
	return parse(buf)
}
