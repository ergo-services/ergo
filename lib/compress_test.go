package lib

import (
	"encoding/binary"
	"testing"
)

var (
	srcCompress string = RandomString(1024)
)

func TestCompressDecompressGZIP(t *testing.T) {
	buf := TakeBuffer()
	buf.AppendString(srcCompress)
	header := uint(12)
	dst, err := CompressGZIP(buf, header, 2)
	if err != nil {
		t.Fatal(err)
	}

	d, err := DecompressGZIP(dst, header, 0)
	if err != nil {
		t.Fatal(err)
	}

	if srcCompress != string(d.B) {
		t.Fatal("incorrect result")
	}
}

func TestCompressDecompressZLIB(t *testing.T) {
	buf := TakeBuffer()
	buf.AppendString(srcCompress)
	header := uint(12)
	dst, err := CompressZLIB(buf, header, 2)
	if err != nil {
		t.Fatal(err)
	}

	d, err := DecompressZLIB(dst, header, 0)
	if err != nil {
		t.Fatal(err)
	}

	if srcCompress != string(d.B) {
		t.Fatal("incorrect result")
	}
}

func TestCompressDecompressLZW(t *testing.T) {
	buf := TakeBuffer()
	buf.AppendString(srcCompress)
	header := uint(12)
	dst, err := CompressLZW(buf, header)
	if err != nil {
		t.Fatal(err)
	}

	d, err := DecompressLZW(dst, header, 0)
	if err != nil {
		t.Fatal(err)
	}

	if srcCompress != string(d.B) {
		t.Fatal("incorrect result")
	}
}

// Pooled decompressors stay correct across reuse, including after a failed stream.
func TestDecompressReuse(t *testing.T) {
	header := uint(12)
	cases := []struct {
		name       string
		compress   func(*Buffer) (*Buffer, error)
		decompress func(*Buffer, uint, int) (*Buffer, error)
	}{
		{"gzip", func(b *Buffer) (*Buffer, error) { return CompressGZIP(b, header, 0) }, DecompressGZIP},
		{"zlib", func(b *Buffer) (*Buffer, error) { return CompressZLIB(b, header, 0) }, DecompressZLIB},
		{"lzw", func(b *Buffer) (*Buffer, error) { return CompressLZW(b, header) }, DecompressLZW},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i := 0; i < 60; i++ {
				src := TakeBuffer()
				src.AppendString(RandomString(128 + i*97))
				z, err := c.compress(src)
				if err != nil {
					t.Fatal(err)
				}

				switch i % 3 {
				case 1:
					// truncated stream
					bad := TakeBuffer()
					bad.Append(z.B[:int(header)+4+(z.Len()-int(header)-4)/2])
					if _, err := c.decompress(bad, header, 0); err == nil {
						t.Fatalf("round %d: truncated stream decompressed without error", i)
					}
					ReleaseBuffer(bad)
				case 2:
					// declared size smaller than the stream
					bad := TakeBuffer()
					bad.Append(z.B)
					binary.BigEndian.PutUint32(bad.B[header:], uint32(src.Len()-1))
					if _, err := c.decompress(bad, header, 0); err == nil {
						t.Fatalf("round %d: short declared size decompressed without error", i)
					}
					ReleaseBuffer(bad)
				}

				d, err := c.decompress(z, header, 0)
				if err != nil {
					t.Fatalf("round %d: %s", i, err)
				}
				if string(d.B) != string(src.B) {
					t.Fatalf("round %d: incorrect result", i)
				}
				ReleaseBuffer(d)
				ReleaseBuffer(z)
				ReleaseBuffer(src)
			}
		})
	}
}

// Once warmed up, gzip and lzw decompression reuse their pooled readers.
func TestDecompressNoAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("sync.Pool drops items at random under the race detector")
	}
	header := uint(12)
	src := TakeBuffer()
	src.AppendString(srcCompress)
	gz, err := CompressGZIP(src, header, 0)
	if err != nil {
		t.Fatal(err)
	}
	lz, err := CompressLZW(src, header)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		z          *Buffer
		decompress func(*Buffer, uint, int) (*Buffer, error)
	}{
		{"gzip", gz, DecompressGZIP},
		{"lzw", lz, DecompressLZW},
	}
	for _, c := range cases {
		allocs := testing.AllocsPerRun(100, func() {
			d, err := c.decompress(c.z, header, 0)
			if err != nil {
				t.Fatal(err)
			}
			ReleaseBuffer(d)
		})
		if allocs > 0 {
			t.Fatalf("%s: %v allocations per decompression, want 0", c.name, allocs)
		}
	}
}

// The level reaches the writer, also when pooled writers of other levels are reused.
func TestCompressLevel(t *testing.T) {
	header := uint(12)
	src := TakeBuffer()
	src.AppendString(srcCompress)

	// gzip XFL byte: 0 default, 4 best speed, 2 best size (an unknown level is default)
	// zlib FLEVEL bits: 2 default, 0 best speed, 3 best size
	levels := []int{2, 1, 0, 2, 1, 0, 5}
	xfl := map[int]byte{0: 0, 1: 4, 2: 2, 5: 0}
	flevel := map[int]byte{0: 2, 1: 0, 2: 3, 5: 2}

	for _, level := range levels {
		gz, err := CompressGZIP(src, header, level)
		if err != nil {
			t.Fatal(err)
		}
		if x := gz.B[header+4+8]; x != xfl[level] {
			t.Fatalf("gzip level %d: XFL = %d, want %d", level, x, xfl[level])
		}
		ReleaseBuffer(gz)

		zl, err := CompressZLIB(src, header, level)
		if err != nil {
			t.Fatal(err)
		}
		if f := zl.B[header+4+1] >> 6; f != flevel[level] {
			t.Fatalf("zlib level %d: FLEVEL = %d, want %d", level, f, flevel[level])
		}
		ReleaseBuffer(zl)
	}
}
