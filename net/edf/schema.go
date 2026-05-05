package edf

import (
	"fmt"
	"reflect"
	"strings"
)

// schemaFor returns a Go-syntax representation of t's structure.
// Structs expand one level (field name + field type by name); nested
// struct fields are referred to by name only.
func schemaFor(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return t.Kind().String()

	case reflect.Slice, reflect.Array, reflect.Map, reflect.Pointer:
		return typeRefName(t)

	case reflect.Struct:
		var sb strings.Builder
		if t.Name() != "" {
			sb.WriteString(t.Name())
			sb.WriteString(" ")
		}
		sb.WriteString("struct {\n")
		nf := t.NumField()
		for i := 0; i < nf; i++ {
			f := t.Field(i)
			sb.WriteString("    ")
			sb.WriteString(f.Name)
			sb.WriteString(" ")
			sb.WriteString(typeRefName(f.Type))
			if path := typeRefPath(f.Type); path != "" {
				sb.WriteString("  // ")
				sb.WriteString(path)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("}")
		return sb.String()

	case reflect.Interface:
		if t.Name() != "" {
			return t.Name()
		}
		return "any"
	}
	return t.String()
}

// typeRefName produces a short reference for a type used inside a schema.
func typeRefName(t reflect.Type) string {
	if t.Name() != "" {
		return t.Name()
	}
	switch t.Kind() {
	case reflect.Slice:
		return "[]" + typeRefName(t.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), typeRefName(t.Elem()))
	case reflect.Map:
		return "map[" + typeRefName(t.Key()) + "]" + typeRefName(t.Elem())
	case reflect.Pointer:
		return "*" + typeRefName(t.Elem())
	}
	return t.String()
}

// typeRefPath returns the fully-qualified #path/Name for a struct field
// type, used as an inline comment in the schema. Empty string for built-ins.
func typeRefPath(t reflect.Type) string {
	if t.Name() != "" && t.PkgPath() != "" {
		return "#" + t.PkgPath() + "/" + t.Name()
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Pointer:
		return typeRefPath(t.Elem())
	case reflect.Map:
		keyP := typeRefPath(t.Key())
		valP := typeRefPath(t.Elem())
		if keyP != "" && valP != "" {
			return "[" + keyP + "]" + valP
		}
		if valP != "" {
			return valP
		}
		if keyP != "" {
			return keyP
		}
	}
	return ""
}
