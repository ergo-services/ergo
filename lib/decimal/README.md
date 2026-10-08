# decimal

A fixed-size decimal number for money and other financial quantities: amounts,
prices, rates, fees. A value is 16 bytes with no pointers, it is passed by value,
and arithmetic does not allocate.

```go
import "ergo.services/ergo/lib/decimal"

price := decimal.RequireFromString("19.99")
total := price.Mul(decimal.NewFromInt(3))                      // 59.97
fee := total.Mul(decimal.RequireFromString("0.015")).Round(2) // 0.9
fmt.Println(total.Sub(fee).StringFixed(2))                    // 59.07
```

## Precision

- value = coefficient * 10^-scale: a 122-bit two's complement coefficient and a
  scale 0..36.
- 36 significant digits are always exact, for example 18 integer digits with 18
  fractional ones. The range is about +-2.6 * 10^36.
- Add, Sub and Mul are exact whenever the result is representable. Otherwise it is
  rounded once, half away from zero, to 36 significant digits. When the integer
  part does not fit, the methods panic and the Checked* methods return
  `ErrOverflow`.
- Div rounds to `DivisionPrecision` (16) places. `DivRound(e, places)` and
  `QuoRound(e, places, mode)` take the precision explicitly; there are seven
  rounding modes.
- `1.5` and `1.50` are equal numbers with different scales: compare with `Equal`
  and `Cmp`, not `==`. `Canonical` gives one representation per number.

## Encodings

- Text and JSON use the decimal string. JSON is written as a string (`"59.97"`)
  and read from a string or a number.
- database/sql: `Value` returns the string. `Scan` accepts a string, bytes,
  integers, floats and other decimal types that implement
  `encoding.TextMarshaler`. `NullDecimal` handles NULL.
- EDF (`MarshalEDF`/`UnmarshalEDF`) and `MarshalBinary` use the binary form:

| bytes | content |
|---|---|
| 0 | scale (bits 0..5), sign (bit 7, 1 = negative), bit 6 is 0 |
| 1 | n, the number of magnitude bytes, 0..16 |
| 2 .. 2+n | the coefficient magnitude, big-endian, without leading zero bytes |

Zero takes 2 bytes, 0.90 takes 3 (`02 01 5a`), any value at most 18.

## Migrating from shopspring/decimal

The method set follows `github.com/shopspring/decimal`, so most code moves over by
changing the import path. Results are the same, except where the exact value needs
more than 36 significant digits, which shopspring keeps and this package rounds.
Other differences:

- the range above; `DivisionPrecision` is a constant;
- no `Pow`, `Ln`, trigonometric functions, `RoundCash`, `NewFromFormattedString`;
- the binary and EDF forms are different. Nodes that exchange decimals have to
  switch together, and binary data already stored has to be converted. The text,
  JSON and SQL forms are the same.

## Performance

Values with 4 decimal places, Apple M4 Max, Go 1.27:

| operation | decimal | shopspring/decimal |
|---|---|---|
| Add | 1.1 ns, 0 allocs | 23.7 ns, 2 allocs |
| Mul | 1.3 ns, 0 allocs | 25.1 ns, 2 allocs |
| Div | 5.6 ns, 0 allocs | 84.1 ns, 7 allocs |
| Round | 2.4 ns, 0 allocs | 55.0 ns, 5 allocs |
| Parse | 5.4 ns, 0 allocs | 29.2 ns, 2 allocs |
| String | 12.0 ns, 1 alloc | 60.2 ns, 2 allocs |

## Testing

Every operation is checked against an exact reference on `math/big.Rat`, and
`FuzzParse` and `FuzzArith` fuzz parsing and arithmetic. `DECIMAL_ITER` sets the
number of random cases per operation:

```
DECIMAL_ITER=5000000 go test ./lib/decimal/
go test -fuzz FuzzArith ./lib/decimal/
```
