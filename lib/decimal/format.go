package decimal

import (
	"encoding/binary"
	"math/bits"
)

type fmtBuf [72]byte

const fmtEnd = 64

func digits8(x uint64) uint64 {
	hi := x / 10000
	v := hi | (x-hi*10000)<<32
	q := (v * 5243 >> 19) & 0x000000FF000000FF
	v = q | (v-q*100)<<16
	t := (v * 103 >> 10) & 0x000F000F000F000F
	return t | (v-t*10)<<8 | 0x3030303030303030
}

func (b *fmtBuf) put8(end int, x uint64) {
	b.store8(end, digits8(x))
}

func (b *fmtBuf) store8(end int, w uint64) {
	i := (end - 8) & 63
	binary.LittleEndian.PutUint64(b[i:i+8], w)
}

func (b *fmtBuf) putFixed(end int, v uint64, w int) {
	for w > 8 {
		b.store8(end, digits8(v%1e8))
		v /= 1e8
		end -= 8
		w -= 8
	}
	b.store8(end, digits8(v))
}

func (b *fmtBuf) putUint(end int, v uint64) int {
	for v >= 1e8 {
		b.store8(end, digits8(v%1e8))
		v /= 1e8
		end -= 8
	}
	b.store8(end, digits8(v))
	return end - ndigits(v)
}

func (b *fmtBuf) putU128(end int, m u128) int {
	if m.hi == 0 {
		return b.putUint(end, m.lo)
	}
	q, r := divPow10x128(m, 19)
	b.putFixed(end, r, 19)
	return b.putUint(end-19, q.lo)
}

func ndigits(v uint64) int {
	n := (bits.Len64(v)*1233)>>12 + 1
	if v < pow10tab64[(n-1)&31] {
		n--
	}
	return max(n, 1)
}

func (b *fmtBuf) format(d Decimal, trim bool) (start, end int) {
	neg, m, s := d.parts()
	end = fmtEnd
	var ip u128
	switch {
	case s == 0:
		ip = m
	case s < 20:
		var f uint64
		ip, f = divPow10x128(m, s)
		if s <= 8 {
			b.store8(end, digits8(f))
		} else {
			b.putFixed(end, f, s)
		}
	default:
		q, f1 := divPow10x128(m, 19)
		var f2 uint64
		ip, f2 = divPow10x128(q, s-19)
		b.putFixed(end, f1, 19)
		b.putFixed(end-19, f2, s-19)
	}
	point := end
	if s > 0 {
		point = end - s - 1
		if trim {
			for end > point+1 && b[(end-1)&63] == '0' {
				end--
			}
			if end == point+1 {
				end = point
			}
		}
		b[point&63] = '.'
	}
	if ip.hi == 0 && ip.lo < 1e8 {
		b.store8(point, digits8(ip.lo))
		start = point - ndigits(ip.lo)
	} else {
		start = b.putU128(point, ip)
	}
	if neg {
		start--
		b[start&63] = '-'
	}
	return start, end
}

// String returns the decimal without trailing zeros in the fraction:
// "1.5" for 1.50, "-0.001", "12".
func (d Decimal) String() string {
	var b fmtBuf
	i, j := b.format(d, true)
	return string(b[i:j])
}

// AppendString appends the String form of d to dst.
func (d Decimal) AppendString(dst []byte) []byte {
	var b fmtBuf
	i, j := b.format(d, true)
	return append(dst, b[i:j]...)
}

// StringScaled returns d with all the digits of its scale: "1.50" for 1.50.
func (d Decimal) StringScaled() string {
	var b fmtBuf
	i, j := b.format(d, false)
	return string(b[i:j])
}

// StringFixed rounds half away from zero to places digits after the point
// and formats exactly that many of them: 1.5.StringFixed(3) is "1.500".
func (d Decimal) StringFixed(places int32) string {
	return string(d.AppendFixed(nil, places, RoundHalfUp))
}

// StringFixedBank is StringFixed rounding half to even.
func (d Decimal) StringFixedBank(places int32) string {
	return string(d.AppendFixed(nil, places, RoundHalfEven))
}

// AppendFixed appends d rounded with the mode to places digits after the
// point, formatting exactly that many of them.
func (d Decimal) AppendFixed(dst []byte, places int32, mode RoundingMode) []byte {
	r := d.RoundMode(places, mode)
	var b fmtBuf
	i, j := b.format(r, false)
	dst = append(dst, b[i:j]...)
	if pad := int(places) - r.scale(); pad > 0 {
		if r.scale() == 0 {
			dst = append(dst, '.')
		}
		for ; pad > 0; pad-- {
			dst = append(dst, '0')
		}
	}
	return dst
}
