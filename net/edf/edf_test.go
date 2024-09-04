package edf

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
