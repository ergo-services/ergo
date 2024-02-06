package edf

import (
	"io"
	"sync"
)

// Options for encoding/decoding
type Options struct {
	AtomCache   *sync.Map // atom => id (encoding), id => atom (decoding)
	AtomMapping *sync.Map // atomX => atomY (encoding/decoding)
	RegCache    *sync.Map // type/name => id (encoding), id => type (for decoding)
	ErrCache    *sync.Map // error => id (for encoder), id => error (for decoder)
	Cache       *sync.Map // common cache (caching reflect.Type => encoder, string([]byte) => decoder)
}

const (
	edtType = byte(130) // 0x82
	edtReg  = byte(131) // 0x83
	edtAny  = byte(132) // 0x84

	edtAtom    = byte(140) // 0x8c
	edtString  = byte(141) // 0x8d
	edtBinary  = byte(142) // 0x8e
	edtFloat32 = byte(143) // 0x8f
	edtFloat64 = byte(144) // 0x90
	edtBool    = byte(145) // 0x91
	edtInt8    = byte(146) // 0x92
	edtInt16   = byte(147) // 0x93
	edtInt32   = byte(148) // 0x94
	edtInt64   = byte(149) // 0x95
	edtInt     = byte(150) // 0x96
	edtUint8   = byte(151) // 0x97
	edtUint16  = byte(152) // 0x98
	edtUint32  = byte(153) // 0x99
	edtUint64  = byte(154) // 0x9a
	edtUint    = byte(155) // 0x9b
	edtError   = byte(156) // 0x9c
	edtSlice   = byte(157) // 0x9d
	edtArray   = byte(158) // 0x9e
	edtMap     = byte(159) // 0x9f

	edtPID       = byte(170) // 0xaa
	edtProcessID = byte(171) // 0xab
	edtAlias     = byte(172) // 0xac
	edtEvent     = byte(173) // 0xad
	edtRef       = byte(174) // 0xae
	edtTime      = byte(175) // 0xaf

	edtNil = byte(255) // 0xff
)

type Marshaler interface {
	MarshalEDF(io.Writer) error
}

type Unmarshaler interface {
	UnmarshalEDF([]byte) error
}
