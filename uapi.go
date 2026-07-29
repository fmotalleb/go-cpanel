package cpanel

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
)

// UAPIResult is the parsed envelope of a UAPI (cPanel API v3) response.
//
// UAPI has two wire formats, both of which are normalised into this type:
//
//   - the /execute/<Module>/<function> format:
//     {"data":..., "errors":[...], "messages":[...], "metadata":{...},
//     "status":1, "warnings":[...]}
//   - the legacy /json-api/cpanel format wraps the above in
//     {"apiversion":3, "module":..., "func":..., "result":{...}}.
//
// Data is unmarshalled into the type parameter of the call. When a function
// has no meaningful documented payload the generated code uses
// json.RawMessage for it; use DecodeData to re-interpret it when needed.
type UAPIResult[T any] struct {
	// Data is the function's payload.
	Data T `json:"data"`

	// Status is the envelope's success flag: 1 on success, 0 on failure.
	Status int `json:"status"`

	// Errors lists fatal errors reported by the function.
	Errors []LooseString `json:"errors"`

	// Messages lists informational messages reported by the function.
	Messages []LooseString `json:"messages"`

	// Warnings lists non-fatal problems reported by the function.
	Warnings []LooseString `json:"warnings"`

	// Metadata carries the envelope metadata (transformed, paginate
	// information, ...). Keys not understood by the client are preserved.
	Metadata map[string]json.RawMessage `json:"metadata"`

	// APIVersion, Module and Func are populated for the legacy
	// /json-api/cpanel wire format.
	APIVersion int    `json:"apiversion"`
	Module     string `json:"module"`
	Func       string `json:"func"`

	// Raw is the raw function payload (the "data" member) as returned by
	// the server, kept for forward compatibility.
	Raw json.RawMessage `json:"-"`
}

// OK reports whether the call succeeded (status == 1).
func (r *UAPIResult[T]) OK() bool { return r != nil && r.Status == 1 }

// ErrorStrings returns the envelope's errors as plain strings.
func (r *UAPIResult[T]) ErrorStrings() []string {
	if r == nil {
		return nil
	}
	return looseStringsToStrings(r.Errors)
}

// WarningStrings returns the envelope's warnings as plain strings.
func (r *UAPIResult[T]) WarningStrings() []string {
	if r == nil {
		return nil
	}
	return looseStringsToStrings(r.Warnings)
}

// MessageStrings returns the envelope's messages as plain strings.
func (r *UAPIResult[T]) MessageStrings() []string {
	if r == nil {
		return nil
	}
	return looseStringsToStrings(r.Messages)
}

// DecodeData re-decodes the raw payload into v. It is mainly useful for
// functions whose payload type is json.RawMessage.
func (r *UAPIResult[T]) DecodeData(v any) error {
	if r == nil || r.Raw == nil {
		return nil
	}
	return json.Unmarshal(r.Raw, v)
}

// uapiEnvelope mirrors both UAPI wire formats using raw fields so the data
// payload can be decoded with caller-supplied types.
type uapiEnvelope struct {
	execute    bool
	status     int
	apiVersion int
	module     string
	function   string
	data       json.RawMessage
	errors     []LooseString
	messages   []LooseString
	warnings   []LooseString
	metadata   map[string]json.RawMessage
}

func parseUAPIEnvelope(body []byte) (*uapiEnvelope, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &Error{Op: "parse UAPI response", Body: truncateForError(body)}
	}

	env := &uapiEnvelope{}
	inner := raw
	if wrapped, ok := raw["result"]; ok && len(wrapped) > 0 && wrapped[0] == '{' {
		// Legacy /json-api/cpanel format: descend into "result".
		var innerMap map[string]json.RawMessage
		if err := json.Unmarshal(wrapped, &innerMap); err == nil {
			env.execute = false
			inner = innerMap
		}
		var s struct {
			APIVersion int    `json:"apiversion"`
			Module     string `json:"module"`
			Func       string `json:"func"`
		}
		_ = json.Unmarshal(body, &s)
		env.apiVersion = s.APIVersion
		env.module = s.Module
		env.function = s.Func
	} else {
		env.execute = true
	}

	get := func(key string) json.RawMessage {
		if v, ok := inner[key]; ok && !jsonIsNull(v) {
			return v
		}
		return nil
	}
	if v := get("status"); v != nil {
		_ = json.Unmarshal(v, &env.status)
	}
	env.data = get("data")
	if v := get("errors"); v != nil {
		env.errors = decodeLooseList(v)
	}
	if v := get("messages"); v != nil {
		env.messages = decodeLooseList(v)
	}
	if v := get("warnings"); v != nil {
		env.warnings = decodeLooseList(v)
	}
	if v := get("metadata"); v != nil {
		_ = json.Unmarshal(v, &env.metadata)
	}
	return env, nil
}

// UAPICall executes a UAPI function against a cPanel server.
//
// module and function select the function (for example "Email", "add_pop");
// method must be the HTTP method documented for the function (the generated
// wrappers supply it automatically). args may be nil, an Args value or a
// struct carrying `cpanel` tags.
//
// The returned result is always non-nil when the server returned a parseable
// envelope; when the envelope reports failure the *Error is non-nil too, so
// both messages and failure details stay accessible:
//
//	res, err := cpanel.UAPICall[any](ctx, c, http.MethodGet, "Email", "list_pops", nil)
//	if err != nil {
//	    logs := res.ErrorStrings() // still usable
//	}
func UAPICall[T any](ctx context.Context, c *Client, method, module, function string, args any) (*UAPIResult[T], error) {
	body, err := c.doRaw(ctx, method, c.endpoint("execute", module, function), args)
	if err != nil {
		return nil, annotateError(err, "uapi "+module+"::"+function)
	}
	return decodeUAPIResult[T](body, module, function)
}

func decodeUAPIResult[T any](body []byte, module, function string) (*UAPIResult[T], error) {
	env, err := parseUAPIEnvelope(body)
	if err != nil {
		return nil, err
	}
	res := &UAPIResult[T]{
		Status:     env.status,
		Errors:     env.errors,
		Messages:   env.messages,
		Warnings:   env.warnings,
		Metadata:   env.metadata,
		APIVersion: env.apiVersion,
		Module:     firstNonEmpty(env.module, module),
		Func:       firstNonEmpty(env.function, function),
		Raw:        env.data,
	}
	if len(env.data) > 0 {
		if err := json.Unmarshal(env.data, &res.Data); err != nil {
			// Type mismatch between documented schema and live data: keep the
			// raw payload accessible instead of failing the call outright.
			var raw json.RawMessage
			if uerr := json.Unmarshal(env.data, &raw); uerr == nil {
				anyRaw := raw
				if setAny(&res.Data, anyRaw) {
					return res, nil
				}
			}
			return nil, &Error{Op: "uapi " + module + "::" + function, Body: "cannot decode data payload into " +
				typeNameOf[T]() + ": " + err.Error()}
		}
	}
	if !res.OK() {
		return res, &Error{
			Op:       "uapi " + module + "::" + function,
			Errors:   looseStringsToStrings(res.Errors),
			Warnings: looseStringsToStrings(res.Warnings),
		}
	}
	return res, nil
}

// UAPI is a convenience entry point matching UAPICall without an explicit
// method; it uses GET, which most UAPI functions employ.
func UAPI[T any](ctx context.Context, c *Client, module, function string, args any) (*UAPIResult[T], error) {
	return UAPICall[T](ctx, c, http.MethodGet, module, function, args)
}

func decodeLooseList(v json.RawMessage) []LooseString {
	// Envelopes normally carry arrays, but tolerate a single string/object.
	if len(v) > 0 && v[0] == '[' {
		var out []LooseString
		if err := json.Unmarshal(v, &out); err == nil {
			return out
		}
	}
	var single LooseString
	if err := json.Unmarshal(v, &single); err == nil && single != "" {
		return []LooseString{single}
	}
	return nil
}

func jsonIsNull(v json.RawMessage) bool {
	for _, b := range v {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case 'n':
			return string(v) == "null"
		default:
			return false
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncateForError(b []byte) string {
	if len(b) > 4096 {
		return string(b[:4096])
	}
	return string(b)
}

func typeNameOf[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}

// setAny attempts to assign value to *out when out is "any-typed".
func setAny(out any, value json.RawMessage) bool {
	switch p := out.(type) {
	case *any:
		if p != nil {
			*p = value
			return true
		}
	case *json.RawMessage:
		if p != nil {
			*p = value
			return true
		}
	}
	return false
}
