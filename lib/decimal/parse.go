package decimal

import "math/bits"

const maxKeep = 76

type text interface {
	~string | ~[]byte
}

// NewFromString parses a decimal: an optional sign, digits with an
// optional point, an optional exponent ("1.5", "-.5", "1e-8", "+12.").
// More than 36 significant digits, or more than MaxScale digits after the
// point, are rounded half away from zero; an integer part beyond 36 digits
// is ErrOverflow.
func NewFromString(s string) (Decimal, error) {
	return parse(s)
}

// NewFromBytes is NewFromString for a byte slice.
func NewFromBytes(b []byte) (Decimal, error) {
	return parse(b)
}

// RequireFromString is NewFromString that panics on error.
func RequireFromString(s string) Decimal {
	return must(parse(s))
}

func load8[T text](s T, i int) uint64 {
	_ = s[i+7]
	return uint64(s[i]) | uint64(s[i+1])<<8 | uint64(s[i+2])<<16 | uint64(s[i+3])<<24 |
		uint64(s[i+4])<<32 | uint64(s[i+5])<<40 | uint64(s[i+6])<<48 | uint64(s[i+7])<<56
}

func eightDigits(v uint64) bool {
	return ((v+0x4646464646464646)|(v-0x3030303030303030))&0x8080808080808080 == 0
}

func value8(v uint64) uint64 {
	const mask = 0x000000FF000000FF
	const mul1 = 0x000F424000000064
	const mul2 = 0x0000271000000001
	v -= 0x3030303030303030
	v = v*10 + v>>8
	return (v&mask*mul1 + (v>>16)&mask*mul2) >> 32
}

func parse[T text](s T) (Decimal, error) {
	n := len(s)
	i := 0
	neg := false
	if n > 0 {
		switch s[0] {
		case '-':
			neg = true
			i = 1
		case '+':
			i = 1
		}
	}
	start := i

	if n-start <= 20 {
		var acc uint64
		point := -1
		for i < n {
			c := s[i] - '0'
			if c < 10 {
				if i+1 < n {
					if c2 := s[i+1] - '0'; c2 < 10 {
						acc = acc*100 + uint64(c)*10 + uint64(c2)
						i += 2
						continue
					}
				}
				acc = acc*10 + uint64(c)
				i++
				continue
			}
			if s[i] == '.' && point < 0 {
				point = i
				i++
				continue
			}
			break
		}
		digits, scale := n-start, 0
		if point >= 0 {
			digits--
			scale = n - point - 1
		}
		if i == n && digits > 0 && digits < 20 {
			return pack(neg, u128{lo: acc}, scale), nil
		}
		return parseSlow(s, start, neg)
	}
	return parseLong(s, start, neg)
}

func parseLong[T text](s T, i int, neg bool) (Decimal, error) {
	n := len(s)
	start := i
	var hi, acc uint64
	cnt := 0
	second := false
	point := -1
	for i < n {
		c := s[i] - '0'
		if c < 10 {
			if cnt <= 17 && i+1 < n {
				if c2 := s[i+1] - '0'; c2 < 10 {
					acc = acc*100 + uint64(c)*10 + uint64(c2)
					cnt += 2
					i += 2
					continue
				}
			}
			if cnt == 19 {
				if second {
					return parseSlow(s, start, neg)
				}
				hi, acc, cnt, second = acc, 0, 0, true
			}
			acc = acc*10 + uint64(c)
			cnt++
			i++
			continue
		}
		if s[i] == '.' && point < 0 {
			point = i
			i++
			continue
		}
		return parseSlow(s, start, neg)
	}
	m := u128{lo: acc}
	if second {
		h, l := bits.Mul64(hi, pow10tab64[cnt])
		var c uint64
		m.lo, c = bits.Add64(l, acc, 0)
		m.hi = h + c
	}
	scale := 0
	if point >= 0 {
		scale = n - point - 1
	}
	if scale <= MaxScale && fits(neg, m) {
		return pack(neg, m, scale), nil
	}
	return parseSlow(s, start, neg)
}

func parseSlow[T text](s T, i int, neg bool) (Decimal, error) {
	n := len(s)
	var (
		big     u256
		acc     uint64
		accN    int
		sig     int
		scale   int
		dot     bool
		digits  bool
		half    = -1
		nz      bool
		dropped bool
	)
	for i < n {
		c := s[i]
		if c == '0' {
			digits = true
			if dot {
				scale++
			}
			i++
			continue
		}
		if c == '.' && dot == false {
			dot = true
			i++
			continue
		}
		break
	}
	for i < n {
		if i+8 <= n && sig+8 <= maxKeep {
			if v := load8(s, i); eightDigits(v) {
				if accN+8 > 19 {
					big = flush(big, acc, accN)
					acc, accN = 0, 0
				}
				acc = acc*1e8 + value8(v)
				accN += 8
				sig += 8
				if dot {
					scale += 8
				}
				digits = true
				i += 8
				continue
			}
		}
		c := s[i]
		if c-'0' < 10 {
			digits = true
			x := int(c - '0')
			if sig < maxKeep {
				if accN == 19 {
					big = flush(big, acc, accN)
					acc, accN = 0, 0
				}
				acc = acc*10 + uint64(x)
				accN++
				sig++
				if dot {
					scale++
				}
			} else {
				if dot == false {
					scale--
				}
				if dropped == false {
					dropped = true
					switch {
					case x > 5:
						half = 1
					case x == 5:
						half = 0
					}
				} else if half == 0 && x != 0 {
					half = 1
				}
				nz = nz || x != 0
			}
			i++
			continue
		}
		if c == '.' && dot == false {
			dot = true
			i++
			continue
		}
		if (c == 'e' || c == 'E') && digits {
			e, ok := parseExp(s, i+1)
			if ok == false {
				return Decimal{}, ErrSyntax
			}
			scale -= e
			i = n
			break
		}
		return Decimal{}, ErrSyntax
	}
	if digits == false {
		return Decimal{}, ErrSyntax
	}
	big = flush(big, acc, accN)
	return finish(neg, big, scale, half, nz, RoundHalfUp)
}

func flush(big u256, acc uint64, accN int) u256 {
	if big.isZero() {
		return u256{w0: acc}
	}
	r, _ := mul256x64(big, pow10tab64[accN])
	var c uint64
	r.w0, c = bits.Add64(r.w0, acc, 0)
	r.w1, c = bits.Add64(r.w1, 0, c)
	r.w2, c = bits.Add64(r.w2, 0, c)
	r.w3 += c
	return r
}

func parseExp[T text](s T, i int) (int, bool) {
	n := len(s)
	neg := false
	if i < n && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	if i == n {
		return 0, false
	}
	e := 0
	for ; i < n; i++ {
		c := s[i]
		if c-'0' >= 10 {
			return 0, false
		}
		if e < 100000 {
			e = e*10 + int(c-'0')
		}
	}
	if neg {
		e = -e
	}
	return e, true
}
