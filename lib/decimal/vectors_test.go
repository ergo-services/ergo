package decimal

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"testing"
)

// TestWriteVectors writes the cross-implementation vectors (the Rust tests
// read them) when DECIMAL_VECTORS names the file:
//
//	DECIMAL_VECTORS=vectors.txt go test -run TestWriteVectors
//
// A decimal is written as coefficient/scale; an error as !.
func TestWriteVectors(t *testing.T) {
	path := os.Getenv("DECIMAL_VECTORS")
	if path == "" {
		t.Skip("DECIMAL_VECTORS is not set")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	enc := func(d Decimal) string {
		return fmt.Sprintf("%s/%d", coef(d), d.scale())
	}
	res := func(d Decimal, err error) string {
		if err != nil {
			return "!"
		}
		return enc(d)
	}
	r := rand.New(rand.NewSource(42))
	const n = 40000
	for i := 0; i < n; i++ {
		a, b := randDecimal(r), randDecimal(r)
		fmt.Fprintf(w, "add %s %s %s\n", enc(a), enc(b), res(a.CheckedAdd(b)))
		fmt.Fprintf(w, "sub %s %s %s\n", enc(a), enc(b), res(a.CheckedSub(b)))
		fmt.Fprintf(w, "mul %s %s %s\n", enc(a), enc(b), res(a.CheckedMul(b)))
		places := DivisionPrecision
		if r.Intn(2) == 0 {
			places = r.Intn(50) - 8
		}
		mode := RoundingMode(r.Intn(7))
		fmt.Fprintf(w, "div %s %s %d %d %s\n", enc(a), enc(b), places, mode, res(a.QuoRound(b, int32(places), mode)))
		fmt.Fprintf(w, "cmp %s %s %d\n", enc(a), enc(b), a.Cmp(b))
		rp := r.Intn(45) - 6
		var rd Decimal
		var rerr error
		func() {
			defer func() {
				if v := recover(); v != nil {
					rerr = v.(error)
				}
			}()
			rd = a.RoundMode(int32(rp), mode)
		}()
		fmt.Fprintf(w, "round %s %d %d %s\n", enc(a), rp, mode, res(rd, rerr))
		fmt.Fprintf(w, "str %s %s %s\n", enc(a), a.String(), a.StringScaled())
		if fp := r.Intn(40) - 4; rerr == nil {
			var s string
			func() {
				defer func() { recover() }()
				s = a.StringFixed(int32(fp))
			}()
			if s != "" {
				fmt.Fprintf(w, "fixed %s %d %s\n", enc(a), fp, s)
			}
		}
		b2, _ := a.MarshalBinary()
		fmt.Fprintf(w, "wire %s %s\n", enc(a), hex.EncodeToString(b2))
		s := randNumberString(r)
		d, err := NewFromString(s)
		fmt.Fprintf(w, "parse %s %s\n", s, res(d, err))
	}
}
