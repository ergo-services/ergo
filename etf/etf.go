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
type Atom string
type Map map[Term]Term

type Pid struct {
	Node     Atom
	ID       uint32
	Serial   uint32
	Creation byte
}

type Port struct {
	Node     Atom
	ID       uint32
	Creation byte
}

type Ref struct {
	Node     Atom
	Creation byte
	ID       []uint32
}

func (ref Ref) String() string {
	return fmt.Sprintf("%#v", ref)
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

func StringTerm(t Term) (s string, ok bool) {
	ok = true
	switch x := t.(type) {
	case Atom:
		s = string(x)
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		ok = false
	}

	return
}

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

	ettList       = byte(108)
	ettSmallTuple = byte(104)
	ettLargeTuple = byte(105)

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

func (p Pid) Str() string {
	hasher32.Write([]byte(p.Node))
	defer hasher32.Reset()
	return fmt.Sprintf("<%X.%d.%d>", hasher32.Sum32(), p.ID, p.Serial)
}

type ProplistElement struct {
	Name  Atom
	Value Term
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

// TermMapIntoSturct transforms etf.Map into the given 'dest'.
// There are limitations to use this helper. A key of the given etf.Map
// must be a string or etf.Atom. 'dest' must be a structure with
// specified tag 'etf' and the name of a key for every single field.
func TermMapIntoStruct(term Term, dest interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	v := reflect.Indirect(reflect.ValueOf(dest))
	return setMapStructField(term.(Map), v)
}

func termIntoStruct(term Term, dest reflect.Value) error {
	t := dest.Type()

	if term == nil {
		return nil
	}

	if t.Kind() == reflect.Interface {
		dest.Set(reflect.ValueOf(term))
		return nil
	}

	switch v := term.(type) {
	case Atom:
		dest.SetString(string(v))
		return nil
	case string:
		dest.SetString(v)
		return nil
	case []byte:
		if t.Kind() == reflect.String {
			dest.SetString(string(v))
		} else if t == reflect.SliceOf(reflect.TypeOf(byte(1))) {
			dest.Set(reflect.ValueOf(v))
		} else {
			return NewInvalidTypesError(t, term)
		}
	case bool:
		dest.SetBool(v)
	case float32:
		dest.SetFloat(float64(v))
	case float64:
		dest.SetFloat(v)
	case int:
		return setIntField(int64(v), dest, t)
	case int8:
		return setIntField(int64(v), dest, t)
	case int16:
		return setIntField(int64(v), dest, t)
	case int32:
		return setIntField(int64(v), dest, t)
	case int64:
		return setIntField(int64(v), dest, t)
	case uint:
		return setUIntField(uint64(v), dest, t)
	case uint8:
		return setUIntField(uint64(v), dest, t)
	case uint16:
		return setUIntField(uint64(v), dest, t)
	case uint32:
		return setUIntField(uint64(v), dest, t)
	case uint64:
		return setUIntField(uint64(v), dest, t)
	case Map:
		return setMapField(v, dest, t)
	case List:
		return setListField(v, dest, t)
	case Tuple:
		return setStructField([]Term(v), dest, t)
	default:
		dest.Set(reflect.ValueOf(v))
	}

	return nil
}

func setListField(term List, dest reflect.Value, t reflect.Type) error {
	var value reflect.Value

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
	for i, _ := range fields {
		fields[i] = t.Field(i)
	}

	for _, elem := range list {
		if len(elem.(Tuple)) != 2 {
			return &InvalidStructKeyError{Term: elem}
		}

		key := elem.(Tuple)[0]
		val := elem.(Tuple)[1]
		fName, ok := StringTerm(key)
		if !ok {
			return &InvalidStructKeyError{Term: key}
		}
		index, _ := findStructField(fields, fName)
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
	for i, _ := range fields {
		fields[i] = t.Field(i)
	}

	for _, elem := range proplist {
		fName, ok := StringTerm(elem.Name)
		if !ok {
			return &InvalidStructKeyError{Term: elem.Name}
		}
		index, _ := findStructField(fields, fName)
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
func setMapField(term Map, dest reflect.Value, t reflect.Type) error {
	switch t.Kind() {
	case reflect.Map:
		return setMapMapField(term, dest, t)
	case reflect.Interface:
		// TODO... do this a better way
		dest.Set(reflect.ValueOf(term))
		return nil
	}

	return NewInvalidTypesError(t, term)
}

func setStructField(term Tuple, dest reflect.Value, t reflect.Type) error {
	if dest.Kind() == reflect.Ptr {
		pdest := reflect.New(dest.Type().Elem())
		dest.Set(pdest)
		dest = pdest.Elem()
	}
	for i, elem := range term {
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
	for i, _ := range fields {
		fields[i] = t.Field(i)
	}

	for key, val := range term {
		fName, ok := StringTerm(key)
		if !ok {
			return &InvalidStructKeyError{Term: key}
		}
		index, _ := findStructField(fields, fName)
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

func findStructField(term []reflect.StructField, key string) (index int, structField reflect.StructField) {
	index = -1
	for i, f := range term {
		tag := f.Tag.Get("etf")
		split := strings.Split(tag, ",")
		if len(split) > 0 && split[0] != "" {
			if split[0] == key {
				return i, f
			}
		} else {
			if strings.EqualFold(f.Name, key) {
				structField = f
				index = i
			}
		}
	}

	return
}

func setMapMapField(term Map, dest reflect.Value, t reflect.Type) error {
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

func setIntField(i int64, field reflect.Value, t reflect.Type) error {
	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		field.SetInt(int64(i))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		field.SetUint(uint64(i))
	default:
		return NewInvalidTypesError(field.Type(), i)
	}
	return nil
}

func setUIntField(ui uint64, field reflect.Value, t reflect.Type) error {
	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		field.SetInt(int64(ui))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		field.SetUint(uint64(ui))
	default:
		return NewInvalidTypesError(field.Type(), ui)
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
