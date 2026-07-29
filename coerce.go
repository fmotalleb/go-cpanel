package cpanel

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// relaxedUnmarshal attempts standard JSON unmarshalling first. When the
// standard decoder fails with a type mismatch (e.g. JSON string supplied
// for an int64 field, which WHM sometimes returns for values like Uid or
// MAX_TEAM_USERS), it falls back to a type-coercion strategy: it decodes
// into generic any, walks the value tree with reflection to reconcile
// types, and then re-encodes and retries the unmarshal.
//
// This makes the decoder more tolerant of cPanel/WHM API responses that
// mix types in ways the documented schemas don't predict.
func relaxedUnmarshal(data []byte, v any) error {
	// Fast path: try standard json.Unmarshal first.
	if err := json.Unmarshal(data, v); err == nil {
		return nil
	}

	// Decode into generic any — this always succeeds for well-formed JSON.
	var intermediate any
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	// Determine the target type for coercion.
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return json.Unmarshal(data, v) // can't coerce into non-pointer/nil
	}
	targetType := val.Type().Elem()

	// Walk the tree, coercing types to match v's target type.
	coerced := coerceToTarget(intermediate, targetType)

	// Re-marshal the coerced value.
	fixed, err := json.Marshal(coerced)
	if err != nil {
		return err
	}

	// Retry the original unmarshal.
	return json.Unmarshal(fixed, v)
}

// coerceToTarget walks a Go value (produced by json.Unmarshal into any) and
// coerces leaf values to match the types expected by target.
//
// For example, a JSON string "12345" is converted to int64(12345) when the
// target field type is int64, and a JSON number 1 is converted to "1" when
// the target field type is string.
func coerceToTarget(value any, target reflect.Type) any {
	if value == nil || target == nil {
		return value
	}

	// Dereference pointer types to get the underlying type.
	for target.Kind() == reflect.Ptr {
		target = target.Elem()
	}

	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Target expects an integer type — try to parse strings.
		if s, ok := value.(string); ok {
			n, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				return n
			}
			// Maybe it's a float-like string ("123.0").
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return int64(f)
			}
		}
		return value

	case reflect.Float32, reflect.Float64:
		// Target expects a float type — try to parse strings.
		if s, ok := value.(string); ok {
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return f
			}
		}
		return value

	case reflect.Bool:
		if s, ok := value.(string); ok {
			b, err := strconv.ParseBool(s)
			if err == nil {
				return b
			}
		}
		return value

	case reflect.String:
		// Target expects a string — convert numbers to their string form.
		switch v := value.(type) {
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			if v {
				return "1"
			}
			return "0"
		}
		return value

	case reflect.Map:
		// Recursively coerce map values.
		if m, ok := value.(map[string]any); ok {
			elemType := target.Elem()
			result := make(map[string]any, len(m))
			for k, v := range m {
				result[k] = coerceToTarget(v, elemType)
			}
			return result
		}
		return value

	case reflect.Slice:
		// Recursively coerce slice elements.
		if s, ok := value.([]any); ok {
			elemType := target.Elem()
			result := make([]any, len(s))
			for i, v := range s {
				result[i] = coerceToTarget(v, elemType)
			}
			return result
		}
		return value

	case reflect.Struct:
		// For struct targets, coerce each field by its JSON-tag-matched type.
		if m, ok := value.(map[string]any); ok {
			fieldTypes := buildFieldTypeMap(target)
			result := make(map[string]any, len(m))
			for k, v := range m {
				if ft, ok := fieldTypes[k]; ok {
					result[k] = coerceToTarget(v, ft)
				} else {
					// Unknown field — keep the original value as-is.
					result[k] = v
				}
			}
			return result
		}
		return value

	default:
		// Interface, json.RawMessage, etc. — no coercion needed.
		return value
	}
}

// buildFieldTypeMap returns a mapping from JSON tag name to field type for
// each exported field in the struct type t, including promoted fields from
// embedded (anonymous) structs.
func buildFieldTypeMap(t reflect.Type) map[string]reflect.Type {
	n := t.NumField()
	m := make(map[string]reflect.Type, n)
	for i := range n {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		// If this is an embedded struct, recursively add its fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for k, ft := range buildFieldTypeMap(f.Type) {
				if _, exists := m[k]; !exists {
					m[k] = ft
				}
			}
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" {
			tag = f.Name
		} else if idx := strings.IndexByte(tag, ','); idx >= 0 {
			tag = tag[:idx]
		}
		if tag == "-" {
			continue
		}
		m[tag] = f.Type
	}
	return m
}
