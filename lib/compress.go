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
	zlibWriters [3]chan *zlib.Writer
	lzwWriters  chan *lzw.Writer

	gzipReaders sync.Pool
	zlibReaders sync.Pool
	lzwReaders  sync.Pool
)

// pooled decompressors, each with its own source reader
type gzipReader struct {
	src bytes.Reader
	r   gzip.Reader
}

type zlibReader struct {
	src bytes.Reader
	r   io.ReadCloser
}

type lzwReader struct {
	src bytes.Reader
	r   lzw.Reader
}

// CompressLZW (LZW has no compression levels)
func CompressLZW(src *Buffer, preallocate uint) (dst *Buffer, err error) {
	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message too large")
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
	if e := zWriter.Close(); err == nil {
		err = e
	}
	select {
	case lzwWriters <- zWriter:
	default:
	}
	if err != nil {
		ReleaseBuffer(zBuffer)
		return nil, err
	}
	return zBuffer, nil
}

// CompressZLIB level: 0 - default, 1 - best speed, 2 - best size
func CompressZLIB(src *Buffer, preallocate uint, level int) (dst *Buffer, err error) {
	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message too large")
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

	var zWriter *zlib.Writer
	select {
	case zWriter = <-zlibWriters[level]:
		zWriter.Reset(zBuffer)
	default:
		zWriter, _ = zlib.NewWriterLevel(zBuffer, lev)
	}
	_, err = zWriter.Write(src.B)
	if e := zWriter.Close(); err == nil {
		err = e
	}
	select {
	case zlibWriters[level] <- zWriter:
	default:
	}
	if err != nil {
		ReleaseBuffer(zBuffer)
		return nil, err
	}
	return zBuffer, nil
}

// CompressGZIP level: 0 - default, 1 - best speed, 2 - best size
func CompressGZIP(src *Buffer, preallocate uint, level int) (dst *Buffer, err error) {
	if src.Len() > math.MaxUint32 {
		return nil, fmt.Errorf("message too large")
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

	var zWriter *gzip.Writer
	select {
	case zWriter = <-gzipWriters[level]:
		zWriter.Reset(zBuffer)
	default:
		zWriter, _ = gzip.NewWriterLevel(zBuffer, lev)
	}
	_, err = zWriter.Write(src.B)
	if e := zWriter.Close(); err == nil {
		err = e
	}
	select {
	case gzipWriters[level] <- zWriter:
	default:
	}
	if err != nil {
		ReleaseBuffer(zBuffer)
		return nil, err
	}
	return zBuffer, nil
}

// DecompressLZW
func DecompressLZW(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	zr, _ := lzwReaders.Get().(*lzwReader)
	if zr == nil {
		zr = &lzwReader{}
	}
	zr.src.Reset(source[4:])
	zr.r.Reset(&zr.src, lzw.LSB, 8)

	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	err = decompress(dst.B, &zr.r)
	zr.src.Reset(nil)
	lzwReaders.Put(zr)
	if err != nil {
		ReleaseBuffer(dst)
		return nil, err
	}
	return dst, nil
}

// DecompressZLIB
func DecompressZLIB(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	zr, _ := zlibReaders.Get().(*zlibReader)
	if zr == nil {
		zr = &zlibReader{}
	}
	zr.src.Reset(source[4:])
	if zr.r == nil {
		zr.r, err = zlib.NewReader(&zr.src)
	} else {
		err = zr.r.(zlib.Resetter).Reset(&zr.src, nil)
	}
	if err != nil {
		zr.src.Reset(nil)
		zlibReaders.Put(zr)
		return nil, err
	}

	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	err = decompress(dst.B, zr.r)
	zr.src.Reset(nil)
	zlibReaders.Put(zr)
	if err != nil {
		ReleaseBuffer(dst)
		return nil, err
	}
	return dst, nil
}

// DecompressGZIP
func DecompressGZIP(src *Buffer, skip uint, limit int) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	if limit > 0 && lenUnpacked > limit {
		return nil, fmt.Errorf("unpacked size %d exceeds limit %d", lenUnpacked, limit)
	}
	zr, _ := gzipReaders.Get().(*gzipReader)
	if zr == nil {
		zr = &gzipReader{}
	}
	zr.src.Reset(source[4:])
	if err := zr.r.Reset(&zr.src); err != nil {
		zr.src.Reset(nil)
		gzipReaders.Put(zr)
		return nil, err
	}

	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	err = decompress(dst.B, &zr.r)
	zr.src.Reset(nil)
	gzipReaders.Put(zr)
	if err != nil {
		ReleaseBuffer(dst)
		return nil, err
	}
	return dst, nil
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
	for i := range zlibWriters {
		zlibWriters[i] = make(chan *zlib.Writer, size)
	}
	lzwWriters = make(chan *lzw.Writer, size)
}
