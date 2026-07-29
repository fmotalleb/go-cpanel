package cpanel

import (
	"context"
	"encoding/json"
	"net/http"
)

// API2Result is the parsed (deprecated) cPanel API 2 JSON envelope.
//
// The wire format is:
//
//	{"apiversion":2, "func":..., "module":...,
//	  "cpanelresult": {"apiversion":2, "data":[{"foo":"bar"}], "error":"",
//	    "event":{"result":1}, "func":..., "module":...,
//	    "preevent":{"result":1}, "postevent":{"result":1}}}
//
// cPanel API 2 is deprecated; consider the UAPI equivalent of the function
// first (see the uapi sub-package).
type API2Result struct {
	// APIVersion is always 2 for cPanel API 2.
	APIVersion int `json:"apiversion"`

	// Func is the executed function.
	Func string `json:"func"`

	// Module is the module the function belongs to.
	Module string `json:"module"`

	// Result is the cpanelresult block.
	Result API2Inner `json:"cpanelresult"`

	// Error carries a top-level error string (rare).
	Error string `json:"error"`
}

// API2Inner is the cpanelresult block of a cPanel API 2 response.
type API2Inner struct {
	// Data is the raw payload. cPanel API 2 modules usually return an array
	// of hashes here, but some return a hash or scalar; use DecodeData to
	// extract it into the shape you need.
	Data json.RawMessage `json:"data"`

	// Error is the module's error string ("" / "NULL" when no error is
	// reported by some very old modules).
	Error string `json:"error"`

	// Event, PreEvent and PostEvent report the hook execution results.
	Event     API2Event `json:"event"`
	PreEvent  API2Event `json:"preevent"`
	PostEvent API2Event `json:"postevent"`

	// Func and Module repeat the executed target.
	Func   string `json:"func"`
	Module string `json:"module"`

	// Extra preserves any other keys of the cpanelresult block.
	Extra map[string]json.RawMessage `json:"-"`
}

// API2Event wraps the "event" style result flags of cPanel API 2.
type API2Event struct {
	Result int `json:"result"`
}

type api2InnerAlias API2Inner

// UnmarshalJSON implements json.Unmarshaler so unknown keys are preserved.
func (r *API2Inner) UnmarshalJSON(b []byte) error {
	var alias api2InnerAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err == nil {
		for _, known := range []string{"data", "error", "event", "preevent", "postevent", "func", "module", "apiversion"} {
			delete(raw, known)
		}
		if len(raw) > 0 {
			alias.Extra = raw
		}
	}
	*r = API2Inner(alias)
	return nil
}

// OK reports whether the call succeeded.
func (r *API2Result) OK() bool {
	return r != nil && r.Result.Event.Result == 1 && r.Result.Error == ""
}

// DecodeData decodes the cpanelresult.data block into v.
func (r *API2Result) DecodeData(v any) error {
	if r == nil || len(r.Result.Data) == 0 {
		return nil
	}
	return json.Unmarshal(r.Result.Data, v)
}

// API2 executes a (deprecated) cPanel API 2 function directly against a
// cPanel server (port 2083 /json-api/cpanel).
//
//	user is the cPanel account to act on; pass "" to use the authenticated
//	account. module and function select the function, args its parameters.
func (c *Client) API2(ctx context.Context, user, module, function string, args Args) (*API2Result, error) {
	return api2Call(ctx, c, user, module, function, args)
}

// WHMAPI2 executes a (deprecated) cPanel API 2 function through the WHM
// proxy (port 2087 /json-api/cpanel), which requires WHM-level credentials
// and the user argument naming the account to act on.
func (c *Client) WHMAPI2(ctx context.Context, user, module, function string, args Args) (*API2Result, error) {
	return api2Call(ctx, c, user, module, function, args)
}

func api2Call(ctx context.Context, c *Client, user, module, function string, args Args) (*API2Result, error) {
	merged := mergeArgs(args, Args{
		"cpanel_jsonapi_funcversion": "2",
		"cpanel_jsonapi_module":      module,
		"cpanel_jsonapi_func":        function,
	})
	if user != "" {
		if m, ok := merged.(Args); ok {
			m["cpanel_jsonapi_user"] = user
			merged = m
		} else if m, ok := merged.(*mergedArgs); ok {
			m.extra["cpanel_jsonapi_user"] = user
		}
	}
	body, err := c.doRaw(ctx, http.MethodGet, c.endpoint("json-api", "cpanel"), merged)
	if err != nil {
		return nil, annotateError(err, "api2 "+module+"::"+function)
	}
	var res API2Result
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, &Error{Op: "parse API2 response for " + module + "::" + function, Body: truncateForError(body)}
	}
	if !res.OK() {
		reason := res.Result.Error
		if reason == "" {
			reason = res.Error
		}
		return &res, &Error{Op: "api2 " + module + "::" + function, Reason: reason}
	}
	return &res, nil
}
