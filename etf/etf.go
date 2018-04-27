package etf

import (
	"fmt"
	"reflect"
	"strings"
)

type cacheFlag struct {
	isNew      bool
	segmentIdx uint8
}

type atomCacheRef struct {
	idx  uint8
	text *string
}

type Context struct {
	atomCache             [2048]*string
	currentCache          []*string
	ConvertBinaryToString bool
	ConvertAtomsToBinary  bool
}

type Term interface{}
type Tuple []Term
type List []Term
type Atom string
type Map map[Term]Term

type Pid struct {
	Node     Atom
	Id       uint32
	Serial   uint32
	Creation byte
}

type Port struct {
	Node     Atom
	Id       uint32
	Creation byte
}

type Ref struct {
	Node     Atom
	Creation byte
	Id       []uint32
}

type Function struct {
	Arity     byte
	Unique    [16]byte
	Index     uint32
	Free      uint32
	Module    Atom
	OldIndex  uint32
	OldUnique uint32
	Pid       Pid
	FreeVars  []Term
}

var (
	MapType = reflect.TypeOf(Map{})
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
	Arity    byte
}

// Erlang external term tags.
const (
	ettAtom          = byte(100)
	ettAtomUTF8      = byte(118) // this is beyond retarded
	ettBinary        = byte(109)
	ettBitBinary     = byte(77)
	ettCachedAtom    = byte(67)
	ettCacheRef      = byte(82)
	ettExport        = byte(113)
	ettFloat         = byte(99)
	ettFun           = byte(117)
	ettInteger       = byte(98)
	ettLargeBig      = byte(111)
	ettLargeTuple    = byte(105)
	ettList          = byte(108)
	ettNewCache      = byte(78)
	ettNewFloat      = byte(70)
	ettNewFun        = byte(112)
	ettNewRef        = byte(114)
	ettNil           = byte(106)
	ettPid           = byte(103)
	ettPort          = byte(102)
	ettRef           = byte(101)
	ettSmallAtom     = byte(115)
	ettSmallAtomUTF8 = byte(119) // this is beyond retarded
	ettSmallBig      = byte(110)
	ettSmallInteger  = byte(97)
	ettSmallTuple    = byte(104)
	ettString        = byte(107)
	ettMap           = byte(116)
)

const (
	// Erlang external term format version
	EtVersion = byte(131)
)

const (
	// Erlang distribution header
	EtDist = byte('D')
)

var tagNames = map[byte]string{
	ettAtom:          "ATOM_EXT",
	ettAtomUTF8:      "ATOM_UTF8_EXT",
	ettBinary:        "BINARY_EXT",
	ettBitBinary:     "BIT_BINARY_EXT",
	ettCachedAtom:    "ATOM_CACHE_REF",
	ettExport:        "EXPORT_EXT",
	ettFloat:         "FLOAT_EXT",
	ettFun:           "FUN_EXT",
	ettInteger:       "INTEGER_EXT",
	ettLargeBig:      "LARGE_BIG_EXT",
	ettLargeTuple:    "LARGE_TUPLE_EXT",
	ettList:          "LIST_EXT",
	ettMap:           "MAP_EXT",
	ettNewCache:      "NEW_CACHE_EXT",
	ettNewFloat:      "NEW_FLOAT_EXT",
	ettNewFun:        "NEW_FUN_EXT",
	ettNewRef:        "NEW_REFERENCE_EXT",
	ettNil:           "NIL_EXT",
	ettPid:           "PID_EXT",
	ettPort:          "PORT_EXT",
	ettRef:           "REFERENCE_EXT",
	ettSmallAtom:     "SMALL_ATOM_EXT",
	ettSmallAtomUTF8: "SMALL_ATOM_UTF8_EXT",
	ettSmallBig:      "SMALL_BIG_EXT",
	ettSmallInteger:  "SMALL_INTEGER_EXT",
	ettSmallTuple:    "SMALL_TUPLE_EXT",
	ettString:        "STRING_EXT",
}

func (t Tuple) Element(i int) Term {
	return t[i-1]
}

func tagName(t byte) (name string) {
	name = tagNames[t]
	if name == "" {
		name = fmt.Sprintf("%d", t)
	}
	return
}

func TermIntoStruct(term Term, dest interface{}) error {
	v := reflect.Indirect(reflect.ValueOf(dest))
	return termIntoStruct(term, v)

}

func termIntoStruct(term Term, destV reflect.Value) error {
	destType := destV.Type()

	if destType.Kind() == reflect.Interface {
		destV.Set(reflect.ValueOf(term))
		return nil
	}

	switch x := term.(type) {
	case Atom:
		return setStringField(string(x), destV, destType)
	case string:
		return setStringField(x, destV, destType)
	case []byte:
		if destType.Kind() == reflect.String {
			destV.SetString(string(x))
		} else if destType == reflect.SliceOf(reflect.TypeOf(byte(1))) {
			destV.Set(reflect.ValueOf(x))
		} else {
			return NewInvalidTypesError(destType, term)
		}
	case Map:
		return setMapField(x, destV, destType)
	case List:
		return setListField(x, destV, destType)
	default:
		return intSwitch(term, destV, destType)
	}

	return nil
}
func intSwitch(term Term, destV reflect.Value, destType reflect.Type) error {
	switch x := term.(type) {
	case int:
		return setIntField(int64(x), destV, destType)
	case int64:
		return setIntField(x, destV, destType)
	case uint:
		return setUIntField(uint64(x), destV, destType)
	case uint64:
		return setUIntField(x, destV, destType)
	default:
		return fmt.Errorf("Unknown term %s, %v", reflect.TypeOf(term).Name(), x)
	}
}

func setStringField(s string, destV reflect.Value, destType reflect.Type) error {
	if destType.Kind() != reflect.String && (s == "" || s == "nil") {
		return intSwitch(0, destV, destType)
	}
	if destType.Kind() != reflect.String {
		return NewInvalidTypesError(destType, Atom(s))
	}

	destV.SetString(s)
	return nil
}

func setListField(v List, field reflect.Value, t reflect.Type) error {
	if t.Kind() != reflect.Slice {
		return NewInvalidTypesError(t, v)
	}

	sliceV := reflect.MakeSlice(t, len(v), len(v))
	for i, elem := range v {
		sliceElement := sliceV.Index(i)
		err := termIntoStruct(elem, sliceElement)
		if err != nil {
			return err
		}
	}
	field.Set(sliceV)
	return nil
}

func setMapField(v Map, field reflect.Value, t reflect.Type) error {
	if t.Kind() == reflect.Map {
		return setMapMapField(v, field, t)
	} else if t.Kind() == reflect.Struct {
		return setMapStructField(v, field, t)
	} else if t.Kind() == reflect.Interface {
		// TODO... do this a better way
		field.Set(reflect.ValueOf(v))
		return nil
	}

	return NewInvalidTypesError(t, v)
}

func setMapStructField(v Map, st reflect.Value, t reflect.Type) error {
	numField := t.NumField()
	fields := make([]reflect.StructField, numField)
	for i, _ := range fields {
		fields[i] = t.Field(i)
	}

	for key, val := range v {
		fName, ok := StringTerm(key)
		if !ok {
			return &InvalidStructKeyError{Term: key}
		}
		index, _ := findStructField(fields, fName)
		if index == -1 {
			continue
		}

		err := termIntoStruct(val, st.Field(index))
		if err != nil {
			return err
		}
	}

	return nil
}

func findStructField(fields []reflect.StructField, key string) (index int, structField reflect.StructField) {
	index = -1
	for i, f := range fields {
		tag := f.Tag.Get("json")
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

func setMapMapField(v Map, field reflect.Value, t reflect.Type) error {
	if field.IsNil() {
		field.Set(reflect.MakeMapWithSize(t, len(v)))
	}
	for key, val := range v {
		field.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(val))
	}
	return nil
}

func setIntField(v int64, field reflect.Value, t reflect.Type) error {

	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		field.SetInt(int64(v))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		field.SetUint(uint64(v))
	default:
		return NewInvalidTypesError(field.Type(), v)
	}

	return nil
}

func setUIntField(v uint64, field reflect.Value, t reflect.Type) error {

	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		field.SetInt(int64(v))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		field.SetUint(uint64(v))
	default:
		return NewInvalidTypesError(field.Type(), v)
	}

	return nil
}

type StructPopulatorError struct {
	Type reflect.Type
	Term Term
}

func (s *StructPopulatorError) Error() string {
	return fmt.Sprintf("Cannot put %v into go value of type %s", s.Term, s.Type.Kind().String())
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
