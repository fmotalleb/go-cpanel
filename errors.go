package cpanel

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Error describes a failed API request.
//
// It is returned in two situations:
//   - the server answered with HTTP status >= 400 (Auth failures, missing
//     function, ...). In that case StatusCode is set and any parseable API
//     error payload is preserved in Errors/Reason.
//   - the API envelope reported failure (UAPI status=0, WHM
//     metadata.result=0). In that case StatusCode is 200 (or 0) and Errors
//     and/or Reason describe the failure.
//
// Inspect it with errors.As:
//
//	var apiErr *cpanel.Error
//	if errors.As(err, &apiErr) {
//	    log.Printf("call failed: %s (%v)", apiErr.Reason, apiErr.Errors)
//	}
type Error struct {
	// Op is the logical API operation that failed, for example
	// "uapi Email::add_pop" or "whm createacct". It may be empty for
	// transport-level failures.
	Op string

	// StatusCode is the HTTP status code of the response, if any.
	StatusCode int

	// Reason is the WHM metadata.reason value, when available.
	Reason string

	// Errors are the error list from the API envelope, when available.
	Errors []string

	// Warnings are the warning list from the API envelope, when available.
	Warnings []string

	// Body is the raw response body (truncated to 4 KiB) when it did not
	// parse as an API envelope. It is useful for debugging misconfigured
	// proxies and similar situations.
	Body string
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("cpanel: ")
	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}
	if e.StatusCode >= 400 {
		fmt.Fprintf(&b, "HTTP %d", e.StatusCode)
	}
	if e.Reason != "" {
		if e.StatusCode >= 400 {
			b.WriteString(" — ")
		}
		b.WriteString(e.Reason)
	}
	if len(e.Errors) > 0 {
		if e.Reason != "" || e.StatusCode >= 400 {
			b.WriteString(": ")
		}
		b.WriteString(strings.Join(e.Errors, "; "))
	}
	if b.Len() == len("cpanel: ") {
		b.WriteString("request failed")
	}
	return b.String()
}

// newHTTPError builds an *Error from a non-2xx HTTP response, attempting to
// salvage any cPanel/WHM-style JSON error payload from the body for error
// reporting.
func newHTTPError(status int, body []byte) *Error {
	e := &Error{StatusCode: status}
	var probe struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
		Errors  []any  `json:"errors"`
		Result  struct {
			Reason   string   `json:"reason"`
			Errors   []string `json:"errors"`
			Message  string   `json:"message"`
			Messages []string `json:"messages"`
		} `json:"result"`
		Metadata struct {
			Reason string `json:"reason"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &probe); err == nil {
		switch {
		case probe.Reason != "":
			e.Reason = probe.Reason
		case probe.Message != "":
			e.Reason = probe.Message
		case probe.Result.Reason != "":
			e.Reason = probe.Result.Reason
		case probe.Metadata.Reason != "":
			e.Reason = probe.Metadata.Reason
		}
		for _, raw := range probe.Errors {
			if s, ok := raw.(string); ok {
				e.Errors = append(e.Errors, s)
			} else if raw != nil {
				if b, err := json.Marshal(raw); err == nil {
					e.Errors = append(e.Errors, string(b))
				}
			}
		}
		e.Errors = append(e.Errors, probe.Result.Errors...)
		if e.Reason == "" && probe.Result.Message != "" {
			e.Reason = probe.Result.Message
		}
	}
	if e.Reason == "" && len(e.Errors) == 0 {
		snippet := body
		if len(snippet) > 4096 {
			snippet = snippet[:4096]
		}
		e.Body = string(snippet)
	}
	return e
}
