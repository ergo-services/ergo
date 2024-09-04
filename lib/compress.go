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
	"sync"
)

var (
	gzipWriters [3]*sync.Pool
	gzipReaders = &sync.Pool{
		New: func() interface{} {
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

	zWriter := lzw.NewWriter(zBuffer, lzw.LSB, 8)
	if _, err := zWriter.Write(src.B); err != nil {
		return nil, err
	}
	zWriter.Close()
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

	zWriter := zlib.NewWriter(zBuffer)
	if _, err := zWriter.Write(src.B); err != nil {
		return nil, err
	}
	zWriter.Close()
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
	if w, ok := gzipWriters[level].Get().(*gzip.Writer); ok {
		zWriter = w
		zWriter.Reset(zBuffer)
	} else {
		zWriter, _ = gzip.NewWriterLevel(zBuffer, lev)
	}
	if _, err := zWriter.Write(src.B); err != nil {
		return nil, err
	}
	zWriter.Close()
	gzipWriters[level].Put(zWriter)
	return zBuffer, nil
}

func DecompressLZW(src *Buffer, skip uint) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
	reader := lzw.NewReader(bytes.NewBuffer(source[4:]), lzw.LSB, 8)
	dst = TakeBuffer()
	dst.Allocate(lenUnpacked)
	if err := decompress(dst.B, reader); err != nil {
		return nil, err
	}
	return
}
func DecompressZLIB(src *Buffer, skip uint) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
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
func DecompressGZIP(src *Buffer, skip uint) (dst *Buffer, err error) {
	if src.Len() < int(skip)+4 {
		return nil, fmt.Errorf("too short source buffer")
	}
	source := src.B[skip:]
	lenUnpacked := int(binary.BigEndian.Uint32(source[:4]))
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
	for i := range gzipWriters {
		gzipWriters[i] = &sync.Pool{
			New: func() interface{} {
				return nil
			},
		}
	}
}
