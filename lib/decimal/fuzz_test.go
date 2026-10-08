package decimal

import (
	"encoding/binary"
	"math/big"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, s := range []string{"0", "-1.5", "1e-8", "123456789012345678901234567890.123456", ".5", "1.", "+0.000", "99999999999999999999.99999999999999999", "1E+30", "--1", "1.2.3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := NewFromString(s)
		valid, small := isPlainNumber(s)
		if valid && small == false {
			return
		}
		if valid == false {
			if err == nil {
				t.Fatalf("%q accepted: %s", s, got.StringScaled())
			}
			return
		}
		c, cs, ovf, ok := refParse(s)
		if ok == false {
			t.Fatalf("reference rejects %q", s)
		}
		check(t, "parse "+s, got, err, c, cs, ovf)
		if err == nil {
			back, err := NewFromString(got.StringScaled())
			if err != nil || back != got {
				t.Fatalf("round trip %q", s)
			}
		}
	})
}

func isPlainNumber(s string) (bool, bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits, dot := 0, false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && dot == false:
			dot = true
		case (c == 'e' || c == 'E') && digits > 0:
			j := i + 1
			if j < len(s) && (s[j] == '+' || s[j] == '-') {
				j++
			}
			if j == len(s) {
				return false, false
			}
			for ; j < len(s); j++ {
				if s[j] < '0' || s[j] > '9' {
					return false, false
				}
			}
			return true, len(s)-i < 7
		default:
			return false, false
		}
	}
	return digits > 0, true
}

func decimalFromBytes(b []byte) Decimal {
	var w [17]byte
	copy(w[:], b)
	hi := binary.LittleEndian.Uint64(w[0:8])
	lo := binary.LittleEndian.Uint64(w[8:16])
	s := int(w[16]) % (MaxScale + 1)
	h := int64(hi) >> (64 - 58)
	return Decimal{hi: uint64(h)<<scaleBits | uint64(s), lo: lo}
}

func FuzzArith(f *testing.F) {
	f.Add([]byte{1, 2, 3}, []byte{4, 5, 6}, uint8(0), int8(16))
	f.Fuzz(func(t *testing.T, x, y []byte, op uint8, places int8) {
		a, b := decimalFromBytes(x), decimalFromBytes(y)
		ra, rb := ratOf(a), ratOf(b)
		switch op % 5 {
		case 0, 1:
			sub := op%5 == 1
			s := max(a.scale(), b.scale())
			r := new(big.Rat)
			if sub {
				r.Sub(ra, rb)
			} else {
				r.Add(ra, rb)
			}
			c, cs, ovf := refFinish(ratScaled(r, -s), s, RoundHalfUp)
			got, err := a.addSlow(b, sub)
			check(t, "add", got, err, c, cs, ovf)
			if ovf == false {
				var fast Decimal
				if sub {
					fast = a.Sub(b)
				} else {
					fast = a.Add(b)
				}
				if fast != got {
					t.Fatal("fast add differs")
				}
			}
		case 2:
			s := a.scale() + b.scale()
			c, cs, ovf := refFinish(ratScaled(new(big.Rat).Mul(ra, rb), -s), s, RoundHalfUp)
			got, err := a.CheckedMul(b)
			check(t, "mul", got, err, c, cs, ovf)
			if ovf == false && a.Mul(b) != got {
				t.Fatal("fast mul differs")
			}
		case 3:
			if b.IsZero() || a.IsZero() {
				return
			}
			mode := RoundingMode(uint8(places) % 7)
			p := min(int(places), MaxScale)
			c, cs, ovf := refFinish(ratScaled(new(big.Rat).Quo(ra, rb), -p), p, mode)
			got, err := a.QuoRound(b, int32(places), mode)
			check(t, "div", got, err, c, cs, ovf)
		case 4:
			if got, want := a.Cmp(b), ra.Cmp(rb); got != want {
				t.Fatalf("cmp %d %d", got, want)
			}
		}
	})
}
