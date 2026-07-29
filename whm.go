package cpanel

import (
	"context"
	"encoding/json"
	"net/http"
)

// WHMResult is the parsed envelope of a WHM API 1 JSON response.
//
// The wire format is:
//
//	{"data": {...}, "metadata": {"command": "...", "reason": "OK",
//	  "result": 1, "version": 1}}
type WHMResult[T any] struct {
	// Data is the function's payload.
	Data T `json:"data"`

	// Metadata carries the call metadata.
	Metadata WHMMetadata `json:"metadata"`

	// Raw is the raw function payload (the "data" member) as returned by
	// the server, kept for forward compatibility.
	Raw json.RawMessage `json:"-"`
}

// WHMMetadata is the metadata section of a WHM API 1 response.
type WHMMetadata struct {
	// Command is the executed WHM API 1 function name.
	Command string `json:"command"`

	// Reason is "OK" on success, or the reason of the failure.
	Reason string `json:"reason"`

	// Result is 1 on success and 0 on failure.
	Result int `json:"result"`

	// Version is the WHM API version (always 1 here).
	Version int `json:"version"`

	// Messages, Errors and Warnings are emitted by some functions.
	Messages []LooseString `json:"messages"`
	Errors   []LooseString `json:"errors"`
	Warnings []LooseString `json:"warnings"`

	// Output carries the detailed output block some functions return
	// (metadata.output.errors / metadata.output.messages).
	Output *WHMOutput `json:"output"`

	// Extra preserves every other metadata key the server returned.
	Extra map[string]json.RawMessage `json:"-"`
}

// WHMOutput is the structured output block nested in some functions'
// metadata.
type WHMOutput struct {
	Messages []LooseString `json:"messages"`
	Errors   []LooseString `json:"errors"`
	Warnings []LooseString `json:"warnings"`
}

type whmMetadataAlias WHMMetadata

// UnmarshalJSON implements json.Unmarshaler so that unknown metadata keys
// are preserved in Extra.
func (m *WHMMetadata) UnmarshalJSON(b []byte) error {
	var alias whmMetadataAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err == nil {
		for _, known := range []string{"command", "reason", "result", "version", "messages", "errors", "warnings", "output"} {
			delete(raw, known)
		}
		if len(raw) > 0 {
			alias.Extra = raw
		}
	}
	*m = WHMMetadata(alias)
	return nil
}

// OK reports whether the call succeeded (metadata.result == 1).
func (r *WHMResult[T]) OK() bool { return r != nil && r.Metadata.Result == 1 }

// DecodeData re-decodes the raw payload into v. It is mainly useful for
// functions whose payload type is json.RawMessage.
func (r *WHMResult[T]) DecodeData(v any) error {
	if r == nil || r.Raw == nil {
		return nil
	}
	return json.Unmarshal(r.Raw, v)
}

// WHMCall executes a WHM API 1 function against a WHM server.
//
// function is the function name (for example "createacct"); method must be
// the HTTP method documented for the function (the generated wrappers supply
// it automatically). args may be nil, an Args value or a struct carrying
// `cpanel` tags. "api.version=1" is always added for you.
func WHMCall[T any](ctx context.Context, c *Client, method, function string, args any) (*WHMResult[T], error) {
	extra := Args{"api.version": "1"}
	body, err := c.doRaw(ctx, method, c.endpoint("json-api", function), mergeArgs(args, extra))
	if err != nil {
		return nil, annotateError(err, "whm "+function)
	}
	return decodeWHMResult[T](body, function)
}

// WHM is a convenience entry point matching WHMCall without an explicit
// method; it uses GET, which most WHM API 1 functions employ.
func WHM[T any](ctx context.Context, c *Client, function string, args any) (*WHMResult[T], error) {
	return WHMCall[T](ctx, c, http.MethodGet, function, args)
}

func decodeWHMResult[T any](body []byte, function string) (*WHMResult[T], error) {
	var env struct {
		Data     json.RawMessage `json:"data"`
		Metadata WHMMetadata     `json:"metadata"`
		Reason   string          `json:"reason"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &Error{Op: "parse WHM response for " + function, Body: truncateForError(body)}
	}
	res := &WHMResult[T]{Metadata: env.Metadata, Raw: env.Data}
	if res.Metadata.Reason == "" {
		res.Metadata.Reason = env.Reason
	}
	if len(env.Data) > 0 && !jsonIsNull(env.Data) {
		if err := json.Unmarshal(env.Data, &res.Data); err != nil {
			anyRaw := env.Data
			if setAny(&res.Data, anyRaw) {
				return res, nil
			}
			return nil, &Error{Op: "whm " + function, Errors: []string{"cannot decode data payload into " +
				typeNameOf[T]() + ": " + err.Error()}}
		}
	}
	if !res.OK() {
		return res, &Error{
			Op:       "whm " + function,
			Reason:   res.Metadata.Reason,
			Errors:   looseStringsToStrings(res.Metadata.Errors),
			Warnings: looseStringsToStrings(res.Metadata.Warnings),
		}
	}
	return res, nil
}

// mergeArgs combines caller-provided args with compulsory extras; caller
// values win on conflict.
func mergeArgs(args any, extra Args) any {
	if args == nil {
		return extra
	}
	if a, ok := args.(Args); ok {
		out := extra.Clone()
		for k, v := range a {
			out[k] = v
		}
		return out
	}
	return &mergedArgs{args: args, extra: extra}
}

// mergedArgs lets EncodeArgs merge a typed struct with a compulsory map.
type mergedArgs struct {
	args  any
	extra Args
}

// annotateError returns err upgraded with an Op label when it is an *Error.
func annotateError(err error, op string) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok && e.Op == "" {
		e.Op = op
	}
	return err
}
