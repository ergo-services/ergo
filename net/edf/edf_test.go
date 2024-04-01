package edf

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"fmt"
	"reflect"
	"testing"
)

type integerCase struct {
	name    string
	integer any
	bin     []byte
}

func integerCases() []integerCase {

	return []integerCase{
		//
		// unsigned integers
		//
		{"uint8::255", uint8(255), []byte{edtUint8, 255}},
		{"uint16::65535", uint16(65535), []byte{edtUint16, 255, 255}},
		{"uint32::4294967295", uint32(4294967295), []byte{edtUint32, 255, 255, 255, 255}},
		{"uint64::18446744073709551615", uint64(18446744073709551615), []byte{edtUint64, 255, 255, 255, 255, 255, 255, 255, 255}},

		// fails on 32bit arch
		{"uint::18446744073709551615", uint(18446744073709551615), []byte{edtUint, 255, 255, 255, 255, 255, 255, 255, 255}},

		//
		// signed integers
		//

		{"int8::-127", int8(-127), []byte{edtInt8, 129}},
		{"int8::127", int8(127), []byte{edtInt8, 127}},
		{"int16::-32767", int16(-32767), []byte{edtInt16, 128, 1}},
		{"int16::32767", int16(32767), []byte{edtInt16, 127, 255}},
		{"int32::-2147483647", int32(-2147483647), []byte{edtInt32, 128, 0, 0, 1}},
		{"int32::2147483647", int32(2147483647), []byte{edtInt32, 127, 255, 255, 255}},
		{"int64::-9223372036854775807", int64(-9223372036854775807), []byte{edtInt64, 128, 0, 0, 0, 0, 0, 0, 1}},
		{"int64::9223372036854775807", int64(9223372036854775807), []byte{edtInt64, 127, 255, 255, 255, 255, 255, 255, 255}},
		// fails on 32bit arch
		{"int::-9223372036854775807", int(-9223372036854775807), []byte{edtInt, 128, 0, 0, 0, 0, 0, 0, 1}},
		// fails on 32bit arch
		{"int::9223372036854775807", int(9223372036854775807), []byte{edtInt, 127, 255, 255, 255, 255, 255, 255, 255}},
	}
}

func TestEnvBug(t *testing.T) {
	type BugInfo struct {
		Env     map[gen.Env]any
		Loggers []gen.LoggerInfo
	}
	in := BugInfo{
		Env: map[gen.Env]any{
			"x": "y",
		},
		// Loggers: []gen.LoggerInfo{
		// 	gen.LoggerInfo{},
		// },
	}
	if err := RegisterTypeOf(in); err != nil {
		panic(err)
	}

	b := lib.TakeBuffer()
	if err := Encode(in, b, Options{}); err != nil {
		t.Fatal(err)
	}
	bout := []byte{
		edtReg,
		0x0, 0x23, // len
		// #ergo.services/ergo/net/edf/BugInfo
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x42, 0x75, 0x67, 0x49,
		0x6e, 0x66, 0x6f,
		//
		edtMap,
		0x0, 0x0, 0x0, 0x1,
		0x0, 0x1, 0x78, 0x8d, 0x0, 0x1, 0x79, 0x9d, 0x0, 0x0, 0x0, 0x1, 0x83, 0x0, 0x22, 0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65, 0x72, 0x67, 0x6f, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x4c, 0x6f, 0x67, 0x67, 0x65, 0x72, 0x49, 0x6e, 0x66, 0x6f, 0x0, 0x0, 0x0, 0x0, 0xff}

	fmt.Printf("bin: %#v\n", b.B)
	value, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(value, in) {
		fmt.Println("exp", in)
		fmt.Println("got", value)
		t.Fatal("incorrect value")
	}

	if bout == nil {

	}

}
