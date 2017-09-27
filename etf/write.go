package etf

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strings"
)

type ErrUnknownType struct {
	t reflect.Type
}

func (c *Context) WriteDist(w io.Writer, _ []Term) (err error) {
	// TODO: now it is just stub dist header, add cache functionality
	_, err = w.Write([]byte{EtDist, 0})
	return
}

func (c *Context) Write(w io.Writer, term interface{}) (err error) {
	switch v := term.(type) {
	case bool:
		err = c.writeBool(w, v)
	case int8, int16, int32, int64, int:
		err = c.writeInt(w, reflect.ValueOf(term).Int())
	case uint8, uint16, uint32, uint64, uintptr, uint:
		err = c.writeUint(w, reflect.ValueOf(term).Uint())
	case *big.Int:
		err = c.writeBigInt(w, v)
	case string:
		err = c.writeBinary(w, []byte(v))
	case []byte:
		err = c.writeBinary(w, v)
	case float64:
		err = c.writeFloat(w, v)
	case float32:
		err = c.writeFloat(w, float64(v))
	case Atom:
		if c.ConvertAtomsToBinary {
			err = c.writeBinary(w, []byte(v))
		} else {
			err = c.writeAtom(w, v)
		}
	case Pid:
		err = c.writePid(w, v)
	case Tuple:
		err = c.writeTuple(w, v)
	case Ref:
		err = c.writeRef(w, v)
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Struct:
			err = c.writeStruct(w, term)
		case reflect.Array, reflect.Slice:
			err = c.writeList(w, term)
		case reflect.Ptr:
			err = c.Write(w, rv.Elem())
		case reflect.Map:
			err = c.writeMap(w, rv)
		default:
			fmt.Println(rv.Type().Name(), "Default")
			err = &ErrUnknownType{rv.Type()}
		}
	}

	return
}

func (e *ErrUnknownType) Error() string {
	return fmt.Sprintf("write: can't encode type \"%s\"", e.t.Name())
}

func (c *Context) writeAtom(w io.Writer, atom Atom) (err error) {
	switch size := len(atom); {
	// case size <= math.MaxUint8:
	// 	// $sL…
	// 	if _, err = w.Write([]byte{ettSmallAtom, byte(size)}); err == nil {
	// 		_, err = io.WriteString(w, string(atom))
	// 	}

	case size <= math.MaxUint16:
		// $dLL…
		_, err = w.Write([]byte{ettAtom, byte(size >> 8), byte(size)})
		if err == nil {
			_, err = io.WriteString(w, string(atom))
		}

	default:
		err = fmt.Errorf("atom is too big (%d bytes)", size)
	}

	return
}

func (c *Context) writeBigInt(w io.Writer, x *big.Int) (err error) {
	sign := 0
	if x.Sign() < 0 {
		sign = 1
	}

	bytes := reverse(new(big.Int).Abs(x).Bytes())

	switch size := int64(len(bytes)); {
	case size <= math.MaxUint8:
		// $nAS…
		_, err = w.Write([]byte{ettSmallBig, byte(size), byte(sign)})

	case size <= math.MaxUint32:
		// $oAAAAS…
		_, err = w.Write([]byte{
			ettLargeBig,
			byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
			byte(sign),
		})

	default:
		err = fmt.Errorf("bad big int size (%d)", size)
	}

	if err == nil {
		_, err = w.Write(bytes)
	}

	return
}

func (c *Context) writeBinary(w io.Writer, bytes []byte) (err error) {
	switch size := int64(len(bytes)); {
	case size <= math.MaxUint32:
		// $mLLLL…
		data := []byte{
			ettBinary,
			byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
		}
		if _, err = w.Write(data); err == nil {
			_, err = w.Write(bytes)
		}

	default:
		err = fmt.Errorf("bad binary size (%d)", size)
	}

	return
}

func (c *Context) writeBool(w io.Writer, b bool) (err error) {
	// $sL…
	if b {
		_, err = w.Write([]byte{ettAtom, 0, 4, 't', 'r', 'u', 'e'})
	} else {
		_, err = w.Write([]byte{ettAtom, 0, 5, 'f', 'a', 'l', 's', 'e'})
	}

	return
}

func (c *Context) writeFloat(w io.Writer, f float64) (err error) {
	if _, err = w.Write([]byte{ettNewFloat}); err == nil {
		fb := math.Float64bits(f)
		_, err = w.Write([]byte{
			byte(fb >> 56), byte(fb >> 48), byte(fb >> 40), byte(fb >> 32),
			byte(fb >> 24), byte(fb >> 16), byte(fb >> 8), byte(fb),
		})
	}
	return
}

func (c *Context) writeInt(w io.Writer, x int64) (err error) {
	switch {
	case x >= 0 && x <= math.MaxUint8:
		// $aI
		_, err = w.Write([]byte{ettSmallInteger, byte(x)})

	case x >= math.MinInt32 && x <= math.MaxInt32:
		// $bIIII
		x := int32(x)
		_, err = w.Write([]byte{
			ettInteger,
			byte(x >> 24), byte(x >> 16), byte(x >> 8), byte(x),
		})

	default:
		err = c.writeBigInt(w, big.NewInt(x))
	}

	return
}

func (c *Context) writeUint(w io.Writer, x uint64) (err error) {
	switch {
	case x <= math.MaxUint8:
		// $aI
		_, err = w.Write([]byte{ettSmallInteger, byte(x)})

	case x <= math.MaxInt32:
		// $bIIII
		_, err = w.Write([]byte{
			ettInteger,
			byte(x >> 24), byte(x >> 16), byte(x >> 8), byte(x),
		})

	default:
		err = c.writeBigInt(w, new(big.Int).SetUint64(x))
	}

	return
}

func (c *Context) writePid(w io.Writer, p Pid) (err error) {
	if _, err = w.Write([]byte{ettPid}); err != nil {
		return
	} else if err = c.writeAtom(w, p.Node); err != nil {
		return
	}

	_, err = w.Write([]byte{
		0, 0, byte(p.Id >> 8), byte(p.Id),
		byte(p.Serial >> 24),
		byte(p.Serial >> 16),
		byte(p.Serial >> 8),
		byte(p.Serial),
		p.Creation,
	})

	return
}

func (c *Context) writeString(w io.Writer, s string) (err error) {
	switch size := len(s); {
	case size <= math.MaxUint16:
		// $kLL…
		_, err = w.Write([]byte{ettString, byte(size >> 8), byte(size)})
		if err == nil {
			_, err = w.Write([]byte(s))
		}

	default:
		err = fmt.Errorf("string is too big (%d bytes)", size)
	}

	return
}

func (c *Context) writeList(w io.Writer, l interface{}) (err error) {
	rv := reflect.ValueOf(l)
	n := rv.Len()
	_, err = w.Write([]byte{
		ettList,
		byte(n >> 24),
		byte(n >> 16),
		byte(n >> 8),
		byte(n),
	})

	if err != nil {
		return
	}

	for i := 0; i < n; i++ {
		v := rv.Index(i).Interface()
		if err = c.Write(w, v); err != nil {
			return
		}
	}

	_, err = w.Write([]byte{ettNil})

	return
}

func (c *Context) writeStruct(w io.Writer, r interface{}) (err error) {
	rv := reflect.ValueOf(r)
	rt := reflect.TypeOf(r)
	n := rv.NumField()
	buf := new(bytes.Buffer)
	arity := uint32(0)

	// Write the key-value pairs to a temporary buffer first
	for i := 0; i < n; i++ {
		if f := rv.Field(i); f.CanInterface() {
			fieldName := rt.Field(i).Name

			jsonTags := rt.Field(i).Tag.Get("json")
			if jsonTags != "" {
				split := strings.SplitN(jsonTags, ",", 2)
				if len(split) > 0 {
					if split[0] != "" {
						fieldName = split[0]
					}
				}
			}

			err = c.Write(buf, Atom(fieldName))
			if err != nil {
				return
			}

			if err = c.Write(buf, f.Interface()); err != nil {
				return
			}

			arity++
		}
	}

	_, err = w.Write([]byte{
		ettMap,
		byte(arity >> 24),
		byte(arity >> 16),
		byte(arity >> 8),
		byte(arity),
	})

	if err == nil {
		_, err = buf.WriteTo(w)
	}

	return
}

func (c *Context) writeMap(w io.Writer, rv reflect.Value) (err error) {

	keys := rv.MapKeys()

	arity := uint32(0)
	buf := new(bytes.Buffer)
	for _, key := range keys {
		err = c.Write(buf, key.Interface())
		if err != nil {
			return
		}

		val := rv.MapIndex(key).Interface()
		err = c.Write(buf, val)
		if err != nil {
			return
		}
		arity++
	}

	_, err = w.Write([]byte{
		ettMap,
		byte(arity >> 24),
		byte(arity >> 16),
		byte(arity >> 8),
		byte(arity),
	})

	if err == nil {
		_, err = buf.WriteTo(w)
	}

	return
}

func (c *Context) writeRef(w io.Writer, ref Ref) (err error) {
	n := len(ref.Id)
	_, err = w.Write([]byte{ettNewRef, byte(n >> 8), byte(n)})
	if err != nil {
		return
	}
	if err = c.writeAtom(w, ref.Node); err != nil {
		return
	}
	if _, err = w.Write([]byte{ref.Creation}); err != nil {
		return
	}
	for _, v := range ref.Id {
		b := []byte{
			byte(v >> 24),
			byte(v >> 16),
			byte(v >> 8),
			byte(v),
		}
		if _, err = w.Write(b); err != nil {
			return
		}
	}

	return
}

func (c *Context) writeTuple(w io.Writer, tuple Tuple) (err error) {
	n := len(tuple)
	if n <= math.MaxUint8 {
		_, err = w.Write([]byte{ettSmallTuple, byte(n)})
	} else {
		_, err = w.Write([]byte{
			ettLargeTuple,
			byte(n >> 24),
			byte(n >> 16),
			byte(n >> 8),
			byte(n),
		})
	}

	if err != nil {
		return
	}

	for _, v := range tuple {
		if err = c.Write(w, v); err != nil {
			return
		}
	}

	return
}

func reverse(b []byte) []byte {
	size := len(b)
	hsize := size >> 1

	for i := 0; i < hsize; i++ {
		b[i], b[size-i-1] = b[size-i-1], b[i]
	}

	return b
}
