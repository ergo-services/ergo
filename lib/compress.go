package lib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
)

var (
	gzipWriters [3]chan *gzip.Writer
	zlibWriters chan *zlib.Writer
	lzwWriters  chan *lzw.Writer
	gzipReaders = &sync.Pool{
		New: func() any {
			return nil
		},
	}
)

// CompressLZW
func CompressLZW(src *Buffer, preallocate uint) (dst *Buffer, err error) {
	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message to large")
	}

	zBuffer := TakeBuffer()
	zBuffer.Allocate(int(preallocate) + 4)
	binary.BigEndian.PutUint32(zBuffer.B[preallocate:], uint32(src.Len()))

	var zWriter *lzw.Writer
	select {
	case zWriter = <-lzwWriters:
		zWriter.Reset(zBuffer, lzw.LSB, 8)
	default:
		zWriter = lzw.NewWriter(zBuffer, lzw.LSB, 8).(*lzw.Writer)
	}
	_, err = zWriter.Write(src.B)
	zWriter.Close()
	select {
	case lzwWriters <- zWriter:
	default:
	}
	if err != nil {
		return nil, err
	}
	return zBuffer, nil
}

// CompressZLIB
func CompressZLIB(src *Buffer, preallocate uint) (dst *Buffer, err error) {
	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message to large")
	}

	zBuffer := TakeBuffer()
	zBuffer.Allocate(int(preallocate) + 4)
	binary.BigEndian.PutUint32(zBuffer.B[preallocate:], uint32(src.Len()))

	var zWriter *zlib.Writer
	select {
	case zWriter = <-zlibWriters:
		zWriter.Reset(zBuffer)
	default:
		zWriter = zlib.NewWriter(zBuffer)
	}
	_, err = zWriter.Write(src.B)
	zWriter.Close()
	select {
	case zlibWriters <- zWriter:
	default:
	}
	if err != nil {
		return nil, err
	}
	return zBuffer, nil
}

// CompressGZIP level: 0 - default, 1 - best speed, 2 - best size
func CompressGZIP(src *Buffer, preallocate uint, level int) (dst *Buffer, err error) {
	var zWriter *gzip.Writer

	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message to large")
	}

	zBuffer := TakeBuffer()
	zBuffer.Allocate(int(preallocate) + 4)
	binary.BigEndian.PutUint32(zBuffer.B[preallocate:], uint32(src.Len()))

	var lev int
	switch level {
	case 2:
		lev = flate.BestCompression
	case 1:
		lev = flate.BestSpeed
	default:
		level = 0
		lev = flate.DefaultCompression
	}
	select {
	case zWriter = <-gzipWriters[level]:
		zWriter.Reset(zBuffer)
	default:
		zWriter, _ = gzip.NewWriterLevel(zBuffer, lev)
	}
	_, err = zWriter.Write(src.B)
	zWriter.Close()
	select {
	case gzipWriters[level] <- zWriter:
	default:
	}
	if err != nil {
		return nil, err
	}
	return zBuffer, nil
}

func DecompressLZW(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	reader := lzw.NewReader(bytes.NewBuffer(source[4:]), lzw.LSB, 8)
	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	if err := decompress(dst.B, reader); err != nil {
		return nil, err
	}
	return
}
func DecompressZLIB(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	reader, err := zlib.NewReader(bytes.NewBuffer(source[4:]))
	if err != nil {
		return nil, err
	}
	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	if err := decompress(dst.B, reader); err != nil {
		return nil, err
	}
	return
}
func DecompressGZIP(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	reader, err := gzip.NewReader(bytes.NewBuffer(source[4:]))
	if err != nil {
		return nil, err
	}
	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)

	if err := decompress(dst.B, reader); err != nil {
		return nil, err
	}
	return
}

func decompress(dst []byte, reader io.Reader) error {
	total := 0
	for {
		n, e := reader.Read(dst[total:])
		total += n
		if e == io.EOF {
			break
		}
		if n == 0 {
			return fmt.Errorf("dst buffer too small")
		}
		if e != nil {
			return e
		}
	}
	if total != len(dst) {
		return fmt.Errorf("unpacked size mismatch")
	}

	return nil
}

func init() {
	size := 4 * runtime.GOMAXPROCS(0)
	if size < 8 {
		size = 8
	}
	for i := range gzipWriters {
		gzipWriters[i] = make(chan *gzip.Writer, size)
	}
	zlibWriters = make(chan *zlib.Writer, size)
	lzwWriters = make(chan *lzw.Writer, size)
}
