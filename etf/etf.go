package etf

import (
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
)

type Term interface{}
type Tuple []Term
type List []Term
type Alias Ref

// ListImproper as a workaround for the Erlang's improper list [a|b]. Intended to be used to interact with Erlang.
type ListImproper []Term

type Atom string
type Map map[Term]Term

// String this type is intended to be used to interact with Erlang. String value encodes as a binary (Erlang type: <<...>>)
type String string

// Charlist this type is intended to be used to interact with Erlang. Charlist value encodes as a list of int32 numbers in order to support Erlang string with UTF-8 symbols on an Erlang side (Erlang type: [...])
type Charlist string

type Pid struct {
	Node     Atom
	ID       uint64
	Creation uint32
}

type Port struct {
	Node     Atom
	ID       uint32
	Creation uint32
}

type Ref struct {
	Node     Atom
	Creation uint32
	ID       [5]uint32
}

// Marshaler interface implemented by types that can marshal themselves into valid ETF binary
// Interface implementation must be over the object e.g. (MyObject) UnmarshalETF:
//
// 	type MyObject struct{}
//
// 	func (m MyObject) MarshalETF() ([]byte, error) {
// 		var encoded []byte
// 		... encoding routine ...
// 		return encoded, nil
// }
//
type Marshaler interface {
	MarshalETF() ([]byte, error)
}

// Unmarshaler interface implemented by types that can unmarshal an ETF binary of themselves.
// Returns error ErrEmpty for []byte{}.
// Interface implementation must be over pointer to the object e.g. (*MyObject) UnmarshalETF:
//
// 	type MyObject struct{}
//
// 	func (m *MyObject) UnmarshalETF(b []byte) error {
// 		var err error
// 		... decoding routine ...
// 		return err
// 	}
type Unmarshaler interface {
	UnmarshalETF([]byte) error
}

type Function struct {
	Arity  byte
	Unique [16]byte
	Index  uint32
	//	Free      uint32
	Module    Atom
	OldIndex  uint32
	OldUnique uint32
	Pid       Pid
	FreeVars  []Term
}

var (
	hasher32 = fnv.New32a()
)

type Export struct {
	Module   Atom
	Function Atom
	Arity    int
}

// Erlang external term tags.
const (
	ettAtom          = byte(100) //deprecated
	ettAtomUTF8      = byte(118)
	ettSmallAtom     = byte(115) //deprecated
	ettSmallAtomUTF8 = byte(119)
	ettString        = byte(107)

	ettCacheRef = byte(82)

	ettNewFloat = byte(70)

	ettSmallInteger = byte(97)
	ettInteger      = byte(98)
	ettLargeBig     = byte(111)
	ettSmallBig     = byte(110)

	ettList         = byte(108)
	ettListImproper = byte(18) // to be able to encode improper lists like [a|b].
	ettSmallTuple   = byte(104)
	ettLargeTuple   = byte(105)

	ettMap = byte(116)

	ettBinary    = byte(109)
	ettBitBinary = byte(77)

	ettNil = byte(106)

	ettPid      = byte(103)
	ettNewPid   = byte(88) // since OTP 23, only when BIG_CREATION flag is set
	ettNewRef   = byte(114)
	ettNewerRef = byte(90) // since OTP 21, only when BIG_CREATION flag is set

	ettExport = byte(113)
	ettFun    = byte(117) // legacy, wont support it here
	ettNewFun = byte(112)

	ettPort    = byte(102)
	ettNewPort = byte(89) // since OTP 23, only when BIG_CREATION flag is set

	// ettRef        = byte(101) deprecated

	ettFloat = byte(99) // legacy
)

func (m Map) Element(k Term) Term {
	return m[k]
}

func (l List) Element(i int) Term {
	return l[i-1]
}

func (t Tuple) Element(i int) Term {
	return t[i-1]
}

func (p Pid) String() string {
	empty := Pid{}
	if p == empty {
		return "<0.0.0>"
	}

	n := uint32(0)
	if p.Node != "" {
		hasher32.Write([]byte(p.Node))
		defer hasher32.Reset()
		n = hasher32.Sum32()
	}
	return fmt.Sprintf("<%X.%d.%d>", n, int32(p.ID>>32), int32(p.ID))
}

func (r Ref) String() string {
	n := uint32(0)
	if r.Node != "" {
		hasher32.Write([]byte(r.Node))
		defer hasher32.Reset()
		n = hasher32.Sum32()
	}
	return fmt.Sprintf("Ref#<%X.%d.%d.%d>", n, r.ID[0], r.ID[1], r.ID[2])
}

func (a Alias) String() string {
	n := uint32(0)
	if a.Node != "" {
		hasher32.Write([]byte(a.Node))
		defer hasher32.Reset()
		n = hasher32.Sum32()
	}
	return fmt.Sprintf("Ref#<%X.%d.%d.%d>", n, a.ID[0], a.ID[1], a.ID[2])
}

type ProplistElement struct {
	Name  Atom
	Value Term
}

// TermToString transforms given term (Atom, []byte, List) to the string
func TermToString(t Term) (s string, ok bool) {
	ok = true
	switch x := t.(type) {
	case Atom:
		s = string(x)
	case string:
		s = x
	case []byte:
		s = string(x)
	case List:
		str, err := convertCharlistToString(x)
		if err != nil {
			ok = false
			return
		}
		s = str
	default:
		ok = false
	}
	return
}

// ProplistIntoStruct transorms given term into the provided struct 'dest'.
// Proplist is the list of Tuple values with two items { Name , Value },
// where Name can be string or Atom and Value must be the same type as
// it has the field of 'dest' struct with the equivalent name. Its also
// accepts []ProplistElement as a 'term' value
func TermProplistIntoStruct(term Term, dest interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	v := reflect.Indirect(reflect.ValueOf(dest))
	return setProplist(term, v)
}

// TermIntoStruct transforms 'term' (etf.Term, etf.List, etf.Tuple, etf.Map) into the
// given 'dest' (could be a struct, map, slice or array). Its a pretty
// expencive operation in terms of CPU usage so you shouldn't use it
// on highload parts of your code. Use manual type casting instead.
func TermIntoStruct(term Term, dest interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	v := reflect.Indirect(reflect.ValueOf(dest))
	err = termIntoStruct(term, v)
	return
}

func termIntoStruct(term Term, dest reflect.Value) error {

	if term == nil {
		return nil
	}

	if dest.Type().NumMethod() > 0 && dest.CanInterface() {
		v := dest
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			v = v.Addr()

			if u, ok := v.Interface().(Unmarshaler); ok {
				b, is_binary := term.([]byte)
				if !is_binary {
					return fmt.Errorf("can't unmarshal value, wront type %s", term)
				}
				return u.UnmarshalETF(b)
			}
		}
	}

	switch dest.Kind() {
	case reflect.Ptr:
		pdest := reflect.New(dest.Type().Elem())
		dest.Set(pdest)
		dest = pdest.Elem()
		return termIntoStruct(term, dest)

	case reflect.Array, reflect.Slice:
		t := dest.Type()
		byte_slice, ok := term.([]byte)
		if t == reflect.SliceOf(reflect.TypeOf(byte(1))) && ok {
			dest.Set(reflect.ValueOf(byte_slice))
			return nil

		}
		if _, ok := term.(List); !ok {
			// in case if term is the golang native type
			dest.Set(reflect.ValueOf(term))
			return nil
		}
		return setListField(term.(List), dest)

	case reflect.Struct:
		switch s := term.(type) {
		case Map:
			return setMapStructField(s, dest)
		case Tuple:
			return setStructField(s, dest)
		case Ref:
			dest.Set(reflect.ValueOf(s))
			return nil
		case Pid:
			dest.Set(reflect.ValueOf(s))
			return nil
		}
		return fmt.Errorf("can't convert %#v to struct", term)

	case reflect.Map:
		if _, ok := term.(Map); !ok {
			// in case if term is the golang native type
			dest.Set(reflect.ValueOf(term))
			return nil
		}
		return setMapField(term.(Map), dest)

	case reflect.Bool:
		b, ok := term.(bool)
		if !ok {
			return fmt.Errorf("can't convert %#v to bool", term)
		}
		dest.SetBool(b)
		return nil

	case reflect.Float32, reflect.Float64:
		f, ok := term.(float64)
		if !ok {
			return fmt.Errorf("can't convert %#v to float64", term)
		}
		dest.SetFloat(f)
		return nil

	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		i := int64(0)
		switch v := term.(type) {
		case int64:
			i = v
		case int32:
			i = int64(v)
		case int16:
			i = int64(v)
		case int8:
			i = int64(v)
		case int:
			i = int64(v)
		case uint64:
			i = int64(v)
		case uint32:
			i = int64(v)
		case uint16:
			i = int64(v)
		case uint8:
			i = int64(v)
		case uint:
			i = int64(v)
		default:
			return fmt.Errorf("can't convert %#v to int64", term)
		}
		dest.SetInt(i)
		return nil

	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		u := uint64(0)
		switch v := term.(type) {
		case uint64:
			u = v
		case uint32:
			u = uint64(v)
		case uint16:
			u = uint64(v)
		case uint8:
			u = uint64(v)
		case uint:
			u = uint64(v)
		case int64:
			u = uint64(v)
		case int32:
			u = uint64(v)
		case int16:
			u = uint64(v)
		case int8:
			u = uint64(v)
		case int:
			u = uint64(v)

		default:
			return fmt.Errorf("can't convert %#v to uint64", term)
		}
		dest.SetUint(u)
		return nil

	case reflect.String:
		switch v := term.(type) {
		case List:
			s, err := convertCharlistToString(v)
			if err != nil {
				return err
			}
			dest.SetString(s)
			return nil
		case []byte:
			dest.SetString(string(v))
			return nil
		case string:
			dest.SetString(v)
			return nil
		case Atom:
			dest.SetString(string(v))
			return nil
		}

	default:
		dest.Set(reflect.ValueOf(term))
		return nil
	}

	return nil
}

func setListField(term List, dest reflect.Value) error {
	var value reflect.Value
	if dest.Kind() == reflect.Ptr {
		pdest := reflect.New(dest.Type().Elem())
		dest.Set(pdest)
		dest = pdest.Elem()
	}
	t := dest.Type()
	switch t.Kind() {
	case reflect.Slice:
		value = reflect.MakeSlice(t, len(term), len(term))
	case reflect.Array:
		if t.Len() != len(term) {
			return NewInvalidTypesError(t, term)
		}
		value = dest
	default:
		return NewInvalidTypesError(t, term)
	}

	for i, elem := range term {
		if err := termIntoStruct(elem, value.Index(i)); err != nil {
			return err
		}
	}

	if t.Kind() == reflect.Slice {
		dest.Set(value)
	}

	return nil
}

func setProplist(term Term, dest reflect.Value) error {
	switch v := term.(type) {
	case []ProplistElement:
		return setProplistElementField(v, dest)
	case List:
		return setProplistField(v, dest)
	default:
		return NewInvalidTypesError(dest.Type(), term)
	}

}

func setProplistField(list List, dest reflect.Value) error {
	t := dest.Type()
	numField := t.NumField()
	fields := make([]reflect.StructField, numField)
	for i := range fields {
		fields[i] = t.Field(i)
	}

	for _, elem := range list {
		if len(elem.(Tuple)) != 2 {
			return &InvalidStructKeyError{Term: elem}
		}

		key := elem.(Tuple)[0]
		val := elem.(Tuple)[1]
		fName, ok := TermToString(key)
		if !ok {
			return &InvalidStructKeyError{Term: key}
		}
		index := findStructField(fields, fName)
		if index == -1 {
			continue
		}

		err := termIntoStruct(val, dest.Field(index))
		if err != nil {
			return err
		}
	}

	return nil
}

func setProplistElementField(proplist []ProplistElement, dest reflect.Value) error {
	t := dest.Type()
	numField := t.NumField()
	fields := make([]reflect.StructField, numField)
	for i := range fields {
		fields[i] = t.Field(i)
	}

	for _, elem := range proplist {
		fName, ok := TermToString(elem.Name)
		if !ok {
			return &InvalidStructKeyError{Term: elem.Name}
		}
		index := findStructField(fields, fName)
		if index == -1 {
			continue
		}

		err := termIntoStruct(elem.Value, dest.Field(index))
		if err != nil {
			return err
		}
	}

	return nil
}
func setMapField(term Map, dest reflect.Value) error {
	switch dest.Type().Kind() {
	case reflect.Map:
		return setMapMapField(term, dest)
	case reflect.Struct:
		return setMapStructField(term, dest)
	case reflect.Interface:
		dest.Set(reflect.ValueOf(term))
		return nil
	}

	return NewInvalidTypesError(dest.Type(), term)
}

func setStructField(term Tuple, dest reflect.Value) error {
	if dest.Kind() == reflect.Ptr {
		pdest := reflect.New(dest.Type().Elem())
		dest.Set(pdest)
		dest = pdest.Elem()
	}
	for i, elem := range term {
		// let it panic if number of term elements is bigger than
		// number of struct fields
		if err := termIntoStruct(elem, dest.Field(i)); err != nil {
			return err
		}
	}

	return nil

}

func setMapStructField(term Map, dest reflect.Value) error {
	t := dest.Type()
	numField := t.NumField()
	fields := make([]reflect.StructField, numField)
	for i := range fields {
		fields[i] = t.Field(i)
	}

	for key, val := range term {
		fName, ok := TermToString(key)
		if !ok {
			return &InvalidStructKeyError{Term: key}
		}
		index := findStructField(fields, fName)
		if index == -1 {
			continue
		}

		err := termIntoStruct(val, dest.Field(index))
		if err != nil {
			return err
		}
	}

	return nil
}

func findStructField(term []reflect.StructField, key string) (index int) {
	var fieldName string
	index = -1
	for i, f := range term {
		fieldName = f.Name

		if tag := f.Tag.Get("etf"); tag != "" {
			fieldName = tag
		}

		if fieldName == key {
			index = i
			return
		} else {
			if strings.EqualFold(f.Name, key) {
				index = i
			}
		}
	}

	return
}

func setMapMapField(term Map, dest reflect.Value) error {
	t := dest.Type()
	if dest.IsNil() {
		dest.Set(reflect.MakeMapWithSize(t, len(term)))
	}
	tkey := t.Key()
	tval := t.Elem()
	for key, val := range term {
		destkey := reflect.Indirect(reflect.New(tkey))
		if err := termIntoStruct(key, destkey); err != nil {
			return err
		}
		destval := reflect.Indirect(reflect.New(tval))
		if err := termIntoStruct(val, destval); err != nil {
			return err
		}
		dest.SetMapIndex(destkey, destval)
	}
	return nil
}

type StructPopulatorError struct {
	Type reflect.Type
	Term Term
}

func (s *StructPopulatorError) Error() string {
	return fmt.Sprintf("Cannot put %#v into go value of type %s", s.Term, s.Type.Kind().String())
}

func NewInvalidTypesError(t reflect.Type, term Term) error {
	return &StructPopulatorError{
		Type: t,
		Term: term,
	}
}

type InvalidStructKeyError struct {
	Term Term
}

func (s *InvalidStructKeyError) Error() string {
	return fmt.Sprintf("Cannot use %s as struct field name", reflect.TypeOf(s.Term).Name())
}

func convertCharlistToString(l List) (string, error) {
	runes := make([]rune, len(l))
	for i := range l {
		switch x := l[i].(type) {
		case int64:
			runes[i] = int32(x)
		case int32:
			runes[i] = int32(x)
		case int16:
			runes[i] = int32(x)
		case int8:
			runes[i] = int32(x)
		case int:
			runes[i] = int32(x)
		default:
			return "", fmt.Errorf("wrong rune %#v", l[i])
		}
	}
	return string(runes), nil
}
