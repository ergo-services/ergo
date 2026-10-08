package decimal

import "math/bits"

type u128 struct {
	hi, lo uint64
}

type u256 struct {
	w0, w1, w2, w3 uint64
}

var (
	pow10tab64  = pow10table64()
	pow10tab128 = pow10table128()
	pow10tab256 = pow10table256()

	pow10shift, pow10norm, pow10recip = pow10reciprocals()
)

func pow10table256() (t [78]u256) {
	p := u256{w0: 1}
	for k := range t {
		t[k] = p
		p, _ = mul256x64(p, 10)
	}
	return t
}

func pow10table128() (t [39]u128) {
	p := pow10table256()
	for k := range t {
		t[k] = u128{hi: p[k].w1, lo: p[k].w0}
	}
	return t
}

func pow10table64() (t [20]uint64) {
	p := uint64(1)
	for k := range t {
		t[k] = p
		p *= 10
	}
	return t
}

func pow10reciprocals() (shift [20]uint, norm, recip [20]uint64) {
	p := pow10table64()
	for k := range p {
		s := uint(bits.LeadingZeros64(p[k]))
		shift[k] = s
		norm[k] = p[k] << s
		recip[k] = reciprocal(norm[k])
	}
	return
}

func reciprocal(d uint64) uint64 {
	v, _ := bits.Div64(^d, ^uint64(0), d)
	return v
}

func div2by1(u1, u0, d, v uint64) (q, r uint64) {
	qh, ql := bits.Mul64(v, u1)
	var c uint64
	ql, c = bits.Add64(ql, u0, 0)
	qh, _ = bits.Add64(qh, u1+1, c)
	r = u0 - qh*d
	if r > ql {
		qh--
		r += d
	}
	if r >= d {
		qh++
		r -= d
	}
	return qh, r
}

func divPow10x128(m u128, k int) (q u128, r uint64) {
	d := pow10tab64[k]
	if m.hi == 0 {
		return u128{lo: m.lo / d}, m.lo % d
	}
	s := pow10shift[k]
	dn, v := pow10norm[k], pow10recip[k]
	if m.hi < d {
		q.lo, r = div2by1(m.hi<<s|m.lo>>(64-s), m.lo<<s, dn, v)
		return q, r >> s
	}
	q.hi, r = div2by1(m.hi>>(64-s), m.hi<<s|m.lo>>(64-s), dn, v)
	q.lo, r = div2by1(r, m.lo<<s, dn, v)
	return q, r >> s
}

func divPow10x256(n u256, k int) (q u256, r uint64) {
	if n.w3|n.w2 == 0 {
		q2, r := divPow10x128(n.lo128(), k)
		return to256(q2), r
	}
	s := pow10shift[k]
	d, v := pow10norm[k], pow10recip[k]
	switch {
	case n.w3 >= pow10tab64[k]:
		r = n.w3 >> (64 - s)
		q.w3, r = div2by1(r, n.w3<<s|n.w2>>(64-s), d, v)
		q.w2, r = div2by1(r, n.w2<<s|n.w1>>(64-s), d, v)
	case n.w3 != 0:
		r = n.w3<<s | n.w2>>(64-s)
		q.w2, r = div2by1(r, n.w2<<s|n.w1>>(64-s), d, v)
	case n.w2 >= pow10tab64[k]:
		r = n.w2 >> (64 - s)
		q.w2, r = div2by1(r, n.w2<<s|n.w1>>(64-s), d, v)
	default:
		r = n.w2<<s | n.w1>>(64-s)
	}
	q.w1, r = div2by1(r, n.w1<<s|n.w0>>(64-s), d, v)
	q.w0, r = div2by1(r, n.w0<<s, d, v)
	return q, r >> s
}

func inc128(x u128) u128 {
	var c uint64
	x.lo, c = bits.Add64(x.lo, 1, 0)
	x.hi += c
	return x
}

func (x u128) isZero() bool {
	return x.hi|x.lo == 0
}

func cmp128(x, y u128) int {
	if x.hi != y.hi {
		if x.hi < y.hi {
			return -1
		}
		return 1
	}
	if x.lo != y.lo {
		if x.lo < y.lo {
			return -1
		}
		return 1
	}
	return 0
}

func (x u256) isZero() bool {
	return x.w0|x.w1|x.w2|x.w3 == 0
}

func (x u256) lo128() u128 {
	return u128{hi: x.w1, lo: x.w0}
}

func to256(x u128) u256 {
	return u256{w0: x.lo, w1: x.hi}
}

func cmp256(x, y u256) int {
	switch {
	case x.w3 != y.w3:
		if x.w3 < y.w3 {
			return -1
		}
		return 1
	case x.w2 != y.w2:
		if x.w2 < y.w2 {
			return -1
		}
		return 1
	case x.w1 != y.w1:
		if x.w1 < y.w1 {
			return -1
		}
		return 1
	case x.w0 != y.w0:
		if x.w0 < y.w0 {
			return -1
		}
		return 1
	}
	return 0
}

func add256(x, y u256) u256 {
	var c uint64
	x.w0, c = bits.Add64(x.w0, y.w0, 0)
	x.w1, c = bits.Add64(x.w1, y.w1, c)
	x.w2, c = bits.Add64(x.w2, y.w2, c)
	x.w3, _ = bits.Add64(x.w3, y.w3, c)
	return x
}

func sub256(x, y u256) u256 {
	var b uint64
	x.w0, b = bits.Sub64(x.w0, y.w0, 0)
	x.w1, b = bits.Sub64(x.w1, y.w1, b)
	x.w2, b = bits.Sub64(x.w2, y.w2, b)
	x.w3, _ = bits.Sub64(x.w3, y.w3, b)
	return x
}

func inc256(x u256) u256 {
	var c uint64
	x.w0, c = bits.Add64(x.w0, 1, 0)
	x.w1, c = bits.Add64(x.w1, 0, c)
	x.w2, c = bits.Add64(x.w2, 0, c)
	x.w3 += c
	return x
}

func mul256x64(x u256, y uint64) (u256, uint64) {
	h0, l0 := bits.Mul64(x.w0, y)
	h1, l1 := bits.Mul64(x.w1, y)
	h2, l2 := bits.Mul64(x.w2, y)
	h3, l3 := bits.Mul64(x.w3, y)
	var c uint64
	l1, c = bits.Add64(l1, h0, 0)
	l2, c = bits.Add64(l2, h1, c)
	l3, c = bits.Add64(l3, h2, c)
	return u256{l0, l1, l2, l3}, h3 + c
}

func mul128(x, y u128) u256 {
	h0, l0 := bits.Mul64(x.lo, y.lo)
	h1, l1 := bits.Mul64(x.lo, y.hi)
	h2, l2 := bits.Mul64(x.hi, y.lo)
	h3, l3 := bits.Mul64(x.hi, y.hi)
	w1, c1 := bits.Add64(h0, l1, 0)
	w2, c2 := bits.Add64(h1, l3, c1)
	w3 := h3 + c2
	w1, c1 = bits.Add64(w1, l2, 0)
	w2, c2 = bits.Add64(w2, h2, c1)
	w3 += c2
	return u256{l0, w1, w2, w3}
}

func mul256x128lo(x u256, y u128) u256 {
	r, _ := mul256x64(x, y.lo)
	if y.hi != 0 {
		t, _ := mul256x64(u256{0, x.w0, x.w1, x.w2}, y.hi)
		r = add256(r, t)
	}
	return r
}

func mulPow10(m u128, k int) u256 {
	switch {
	case k == 0:
		return to256(m)
	case k < 20:
		p := pow10tab64[k]
		h0, l0 := bits.Mul64(m.lo, p)
		h1, l1 := bits.Mul64(m.hi, p)
		w1, c := bits.Add64(h0, l1, 0)
		return u256{l0, w1, h1 + c, 0}
	case k < len(pow10tab128):
		return mul128(m, pow10tab128[k])
	}
	return mul256x128lo(pow10tab256[k], m)
}

func bitlen128(x u128) int {
	if x.hi != 0 {
		return 128 - bits.LeadingZeros64(x.hi)
	}
	return 64 - bits.LeadingZeros64(x.lo)
}

func bitlen256(x u256) int {
	switch {
	case x.w3 != 0:
		return 256 - bits.LeadingZeros64(x.w3)
	case x.w2 != 0:
		return 192 - bits.LeadingZeros64(x.w2)
	case x.w1 != 0:
		return 128 - bits.LeadingZeros64(x.w1)
	}
	return 64 - bits.LeadingZeros64(x.w0)
}

func shl256(x u256, n uint) u256 {
	w := [4]uint64{x.w0, x.w1, x.w2, x.w3}
	var r [4]uint64
	ws, bs := int(n/64), n%64
	for i := 3; i >= ws; i-- {
		r[i] = w[i-ws] << bs
		if bs != 0 && i > ws {
			r[i] |= w[i-ws-1] >> (64 - bs)
		}
	}
	return u256{r[0], r[1], r[2], r[3]}
}

func shr256(x u256, n uint) (u256, int, bool) {
	if n == 0 {
		return x, -1, false
	}
	if n >= 256 {
		return u256{}, -1, x.isZero() == false
	}
	w := [4]uint64{x.w0, x.w1, x.w2, x.w3}
	hw, hs := (n-1)/64, (n-1)%64
	top := w[hw]>>hs&1 == 1
	below := w[hw]&(1<<hs-1) != 0
	for i := uint(0); i < hw; i++ {
		below = below || w[i] != 0
	}
	half := -1
	if top {
		half = 0
		if below {
			half = 1
		}
	}
	var r [4]uint64
	ws, bs := int(n/64), n%64
	for i := 0; i+ws < 4; i++ {
		r[i] = w[i+ws] >> bs
		if bs != 0 && i+ws+1 < 4 {
			r[i] |= w[i+ws+1] << (64 - bs)
		}
	}
	return u256{r[0], r[1], r[2], r[3]}, half, top || below
}

func digits128(x u128) int {
	t := bitlen128(x) * 1233 >> 12
	if cmp128(x, pow10tab128[t]) >= 0 {
		t++
	}
	return t
}

func digits256(x u256) int {
	t := bitlen256(x) * 1233 >> 12
	if cmp256(x, pow10tab256[t]) >= 0 {
		t++
	}
	return t
}

func divmod256x64(n u256, d uint64) (q u256, r uint64) {
	if n.w3|n.w2 == 0 {
		if n.w1 < d {
			q.w0, r = bits.Div64(n.w1, n.w0, d)
			return q, r
		}
		q.w1, r = n.w1/d, n.w1%d
		q.w0, r = bits.Div64(r, n.w0, d)
		return q, r
	}
	s := uint(bits.LeadingZeros64(d))
	dn := d << s
	v := reciprocal(dn)
	r = n.w3 >> (64 - s)
	q.w3, r = div2by1(r, n.w3<<s|n.w2>>(64-s), dn, v)
	q.w2, r = div2by1(r, n.w2<<s|n.w1>>(64-s), dn, v)
	q.w1, r = div2by1(r, n.w1<<s|n.w0>>(64-s), dn, v)
	q.w0, r = div2by1(r, n.w0<<s, dn, v)
	return q, r >> s
}

func knuthStep(u2, u1, u0, v1, v0, recip uint64) (q, r1, r0 uint64) {
	var rhat uint64
	rover := false
	switch {
	case u2 >= v1:
		q = ^uint64(0)
		var c uint64
		rhat, c = bits.Add64(u1, v1, 0)
		rover = c != 0
	case recip != 0:
		q, rhat = div2by1(u2, u1, v1, recip)
	default:
		q, rhat = bits.Div64(u2, u1, v1)
	}
	for rover == false {
		ph, pl := bits.Mul64(q, v0)
		if ph < rhat || (ph == rhat && pl <= u0) {
			break
		}
		q--
		var c uint64
		rhat, c = bits.Add64(rhat, v1, 0)
		rover = c != 0
	}
	p0h, p0l := bits.Mul64(q, v0)
	p1h, p1l := bits.Mul64(q, v1)
	m1, c := bits.Add64(p1l, p0h, 0)
	m2 := p1h + c
	var b uint64
	u0, b = bits.Sub64(u0, p0l, 0)
	u1, b = bits.Sub64(u1, m1, b)
	_, b = bits.Sub64(u2, m2, b)
	if b != 0 {
		q--
		u0, c = bits.Add64(u0, v0, 0)
		u1, _ = bits.Add64(u1, v1, c)
	}
	return q, u1, u0
}

func divmod256x128(n u256, d u128) (q u256, r u128) {
	if d.hi == 0 {
		var rr uint64
		q, rr = divmod256x64(n, d.lo)
		return q, u128{lo: rr}
	}
	s := uint(bits.LeadingZeros64(d.hi))
	v1 := d.hi<<s | d.lo>>(64-s)
	v0 := d.lo << s
	u4 := n.w3 >> (64 - s)
	u3 := n.w3<<s | n.w2>>(64-s)
	u2 := n.w2<<s | n.w1>>(64-s)
	u1 := n.w1<<s | n.w0>>(64-s)
	u0 := n.w0 << s
	switch {
	case n.w3 != 0:
		recip := reciprocal(v1)
		q.w2, u3, u2 = knuthStep(u4, u3, u2, v1, v0, recip)
		q.w1, u2, u1 = knuthStep(u3, u2, u1, v1, v0, recip)
		q.w0, u1, u0 = knuthStep(u2, u1, u0, v1, v0, recip)
	case n.w2 != 0:
		recip := reciprocal(v1)
		q.w1, u2, u1 = knuthStep(u3, u2, u1, v1, v0, recip)
		q.w0, u1, u0 = knuthStep(u2, u1, u0, v1, v0, recip)
	default:
		if n.w1 < d.hi || (n.w1 == d.hi && n.w0 < d.lo) {
			return q, n.lo128()
		}
		q.w0, u1, u0 = knuthStep(u2, u1, u0, v1, v0, 0)
	}
	return q, u128{hi: u1 >> s, lo: u0>>s | u1<<(64-s)}
}
