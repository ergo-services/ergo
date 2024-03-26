package lib

import (
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

	d, err := DecompressGZIP(dst, header)
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
	dst, err := CompressZLIB(buf, header)
	if err != nil {
		t.Fatal(err)
	}

	d, err := DecompressZLIB(dst, header)
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

	d, err := DecompressLZW(dst, header)
	if err != nil {
		t.Fatal(err)
	}

	if srcCompress != string(d.B) {
		t.Fatal("incorrect result")
	}
}
