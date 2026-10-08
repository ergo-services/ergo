package decimal

import "testing"

var (
	bSmallX     = RequireFromString("12345.6789")
	bSmallY     = RequireFromString("1.2345")
	bDiffY      = RequireFromString("0.12")
	bScale18X   = RequireFromString("1234.567890123456789012")
	bScale18Y   = RequireFromString("2500.123456789012345678")
	bSink       Decimal
	bSinkStr    string
	bSinkInt    int
	bSinkErr    error
	bSmallStr   = "12345.6789"
	bScale18Str = "1234.567890123456789012"
)

func BenchmarkAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bSmallX.Add(bSmallY)
	}
}

func BenchmarkAddDiff(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bSmallX.Add(bDiffY)
	}
}

func BenchmarkMul(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bSmallX.Mul(bSmallY)
	}
}

func BenchmarkMulScale18(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bScale18X.Mul(bScale18Y)
	}
}

func BenchmarkDiv(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bSmallX.Div(bSmallY)
	}
}

func BenchmarkDivScale18(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bScale18X.Div(bScale18Y)
	}
}

func BenchmarkRound(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bSmallX.Round(2)
	}
}

func BenchmarkRoundScale18(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink = bScale18X.Round(2)
	}
}

func BenchmarkCmp(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSinkInt += bSmallX.Cmp(bSmallY)
	}
}

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink, bSinkErr = NewFromString(bSmallStr)
	}
}

func BenchmarkParseScale18(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSink, bSinkErr = NewFromString(bScale18Str)
	}
}

func BenchmarkString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSinkStr = bSmallX.String()
	}
}

func BenchmarkStringScale18(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bSinkStr = bScale18X.String()
	}
}

func BenchmarkAppendString(b *testing.B) {
	var buf [64]byte
	for i := 0; i < b.N; i++ {
		bSinkInt += len(bSmallX.AppendString(buf[:0]))
	}
}
