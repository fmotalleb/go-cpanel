package cpanel

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Args is a free-form set of API arguments.
//
// Every generated argument struct contains an Extra field of this type, so
// callers can always supply parameters that are not modelled by the struct
// (for example UAPI/WHM meta arguments such as api.filter.*, api.sort.* and
// api.paginate.*).
type Args map[string]string

// Set sets a single argument and returns the receiver, allowing calls to be
// chained.
func (a Args) Set(key, value string) Args {
	a[key] = value
	return a
}

// Clone returns a shallow copy of the map.
func (a Args) Clone() Args {
	out := make(Args, len(a))
	for k, v := range a {
		out[k] = v
	}
	return out
}

// CombineArgs merges several Args maps into one (later maps win on key
// conflicts). It is used by the generated wrappers for functions without
// documented parameters, and is handy for building calls by hand.
func CombineArgs(list ...Args) Args {
	var out Args
	for _, a := range list {
		if a == nil {
			continue
		}
		if out == nil {
			out = make(Args, len(a))
		}
		for k, v := range a {
			out[k] = v
		}
	}
	return out
}

// String returns a pointer to v. It is a convenience for populating the
// optional (pointer) fields of the generated argument structs.
func String[T ~string](v T) *T { return &v }

// Int returns a pointer to v. It is a convenience for populating the
// optional (pointer) fields of the generated argument structs.
func Int[T ~int | ~int64](v T) *T { return &v }

// Float returns a pointer to v. It is a convenience for populating the
// optional (pointer) fields of the generated argument structs.
func Float[T ~float32 | ~float64](v T) *T { return &v }

// Bool returns a pointer to v. It is a convenience for populating the
// optional (pointer) fields of the generated argument structs.
func Bool[T ~bool](v T) *T { return &v }

// EncodeArgs converts an argument value into URL query values.
//
// The value may be one of:
//   - nil,
//   - an Args map,
//   - a *Args map,
//   - a struct (or pointer to struct) whose exported fields carry
//     `cpanel:"name"` tags.
//
// For structs:
//   - required fields are tagged `cpanel:"name"` and are always encoded,
//   - optional fields are tagged `cpanel:"name,omitempty"`; optional scalar
//     fields are pointers so that a nil pointer means "do not send",
//   - a field tagged `cpanel:"-"` is skipped; a field of type Args with that
//     tag is used as the catch-all Extra bag and merged into the output,
//   - slices are encoded as repeated query parameters (OpenAPI form style),
//     except []byte which is sent as a single string,
//   - any value implementing fmt.Stringer is encoded via its String method.
//
// encodeByBuiltinType handles the well-known argument types (Args, *Args,
// url.Values, *mergedArgs) and returns handled=true if v was one of them.
func encodeByBuiltinType(v any) (url.Values, bool, error) {
	switch t := v.(type) {
	case Args:
		values := url.Values{}
		for k, val := range t {
			values.Add(k, val)
		}
		return values, true, nil
	case *Args:
		values := url.Values{}
		if t == nil {
			return values, true, nil
		}
		for k, val := range *t {
			values.Add(k, val)
		}
		return values, true, nil
	case url.Values:
		values := url.Values{}
		for k, vs := range t {
			for _, val := range vs {
				values.Add(k, val)
			}
		}
		return values, true, nil
	case *mergedArgs:
		values := url.Values{}
		if t == nil {
			return values, true, nil
		}
		main, err := EncodeArgs(t.args)
		if err != nil {
			return nil, false, err
		}
		extra, err := EncodeArgs(t.extra)
		if err != nil {
			return nil, false, err
		}
		for k, vs := range main {
			values[k] = vs
		}
		for k, vs := range extra {
			if _, has := values[k]; !has {
				values[k] = vs
			}
		}
		return values, true, nil
	}
	return nil, false, nil
}

// EncodeArgs converts an argument value into URL query values.
//
// The value may be one of:
//   - nil,
//   - an Args map,
//   - a *Args map,
//   - a struct (or pointer to struct) whose exported fields carry
//     `cpanel:"name"` tags.
//
// For structs:
//   - required fields are tagged `cpanel:"name"` and are always encoded,
//   - optional fields are tagged `cpanel:"name,omitempty"`; optional scalar
//     fields are pointers so that a nil pointer means "do not send",
//   - a field tagged `cpanel:"-"` is skipped; a field of type Args with that
//     tag is used as the catch-all Extra bag and merged into the output,
//   - slices are encoded as repeated query parameters (OpenAPI form style),
//     except []byte which is sent as a single string,
//   - any value implementing fmt.Stringer is encoded via its String method.
func EncodeArgs(v any) (url.Values, error) {
	values := url.Values{}
	if v == nil {
		return values, nil
	}

	if result, handled, err := encodeByBuiltinType(v); handled {
		return result, err
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return values, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cpanel: cannot encode arguments of type %T; want struct or Args", v)
	}
	if err := encodeStructValues(values, rv); err != nil {
		return nil, err
	}
	return values, nil
}

func encodeStructValues(values url.Values, rv reflect.Value) error {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("cpanel")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			// Extra Args bag (cpanel:"-").
			if field.Type == reflect.TypeOf(Args(nil)) {
				if a, ok := fv.Interface().(Args); ok {
					for k, v := range a {
						values.Add(k, v)
					}
				}
			} else if field.Type == reflect.TypeOf((*Args)(nil)) {
				if p, ok := fv.Interface().(*Args); ok && p != nil {
					for k, v := range *p {
						values.Add(k, v)
					}
				}
			}
			continue
		}
		omitempty := strings.Contains(opts, "omitempty")
		if err := encodeField(values, name, fv, omitempty); err != nil {
			return fmt.Errorf("cpanel: encoding argument %q: %w", name, err)
		}
	}
	return nil
}

func encodeField(values url.Values, name string, fv reflect.Value, omitempty bool) error {
	// Pointer optionals: nil means "absent".
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			if omitempty {
				return nil
			}
			values.Add(name, "")
			return nil
		}
		// Dereference all but *Args Extra bags (handled by caller).
		if fv.Type() == reflect.TypeOf((*Args)(nil)) {
			return nil
		}
		fv = fv.Elem()
	}

	if fv.CanInterface() {
		if s, ok := fv.Interface().(fmt.Stringer); ok {
			values.Add(name, s.String())
			return nil
		}
	}

	return encodeByKind(values, name, fv, omitempty)
}

func encodeByKind(values url.Values, name string, fv reflect.Value, omitempty bool) error {
	switch fv.Kind() {
	case reflect.String:
		if omitempty && fv.Len() == 0 {
			return nil
		}
		values.Add(name, fv.String())
	case reflect.Bool:
		b := fv.Bool()
		if omitempty && !b {
			return nil
		}
		values.Add(name, boolString(b))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := fv.Int()
		if omitempty && i == 0 {
			return nil
		}
		values.Add(name, strconv.FormatInt(i, 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := fv.Uint()
		if omitempty && u == 0 {
			return nil
		}
		values.Add(name, strconv.FormatUint(u, 10))
	case reflect.Float32, reflect.Float64:
		f := fv.Float()
		if omitempty && f == 0 {
			return nil
		}
		values.Add(name, strconv.FormatFloat(f, 'f', -1, 64))
	case reflect.Slice, reflect.Array:
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.Uint8 {
			b := fv.Bytes()
			if omitempty && len(b) == 0 {
				return nil
			}
			values.Add(name, string(b))
			return nil
		}
		for j := 0; j < fv.Len(); j++ {
			el := fv.Index(j)
			if el.Kind() == reflect.Pointer {
				if el.IsNil() {
					continue
				}
				el = el.Elem()
			}
			if !el.CanInterface() {
				continue
			}
			values.Add(name, fmt.Sprint(el.Interface()))
		}
	case reflect.Interface:
		if fv.IsNil() {
			if omitempty {
				return nil
			}
			values.Add(name, "")
			return nil
		}
		// Unwrap pointers stored inside the interface so that helpers such
		// as cpanel.Int can also populate `any` fields.
		ev := fv.Elem()
		for ev.Kind() == reflect.Pointer {
			if ev.IsNil() {
				if omitempty {
					return nil
				}
				values.Add(name, "")
				return nil
			}
			ev = ev.Elem()
		}
		switch ev.Kind() {
		case reflect.Map, reflect.Slice, reflect.Struct:
			b, err := marshalJSON(fv.Interface())
			if err != nil {
				return err
			}
			values.Add(name, string(b))
		default:
			values.Add(name, fmt.Sprint(ev.Interface()))
		}
		return nil
	case reflect.Map, reflect.Struct:
		// Nested structured values are sent as JSON by convention; very few
		// cPanel & WHM functions need them (e.g. UAPI Batch).
		b, err := marshalJSON(fv.Interface())
		if err != nil {
			return err
		}
		values.Add(name, string(b))
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

func boolString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
