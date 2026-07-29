package cpanel_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	cpanel "github.com/fmotalleb/go-cpanel"
)

func testServer(t *testing.T, h http.HandlerFunc) (*cpanel.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := cpanel.NewClient(srv.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func lastRequest(t *testing.T, h http.HandlerFunc) (*cpanel.Client, func() *http.Request) {
	t.Helper()
	var req *http.Request
	c, srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		req = r.Clone(r.Context())
		h(w, r)
	})
	t.Cleanup(srv.Close)
	return c, func() *http.Request { return req }
}

func TestAuthenticationHeaders(t *testing.T) {
	tests := []struct {
		name string
		auth cpanel.Authenticator
		want string
	}{
		{
			"basic", cpanel.BasicAuth("user", "pass"),
			"Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		},
		{"cpanel token", cpanel.CPanelTokenAuth("user", "TOKEN123"), "cpanel user:TOKEN123"},
		{"whm token", cpanel.WHMTokenAuth("root", "APITOKEN"), "whm root:APITOKEN"},
		{"access hash", cpanel.AccessHashAuth("root", "HASHHASH"), "whm root:HASHHASH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"status":1,"data":{}}`)
			}))
			defer srv.Close()
			c, err := cpanel.NewClient(srv.URL, tt.auth)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cpanel.UAPICall[any](context.Background(), c, http.MethodGet, "Email", "list_pops", nil); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Authorization header = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUAPICallExecuteFormat(t *testing.T) {
	c, getReq := lastRequest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute/Email/add_pop" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiversion": 3,
			"func":       "add_pop",
			"module":     "Email",
			"result": map[string]any{
				"data":     map[string]any{"email": "bob@example.com"},
				"errors":   nil,
				"messages": []string{"created"},
				"metadata": map[string]any{},
				"status":   1,
				"warnings": nil,
			},
		})
	})
	type data struct {
		Email string `json:"email"`
	}
	res, err := cpanel.UAPICall[data](context.Background(), c, http.MethodGet, "Email", "add_pop",
		cpanel.Args{"email": "bob", "domain": "example.com", "password": "s3cret", "quota": "250"})
	if err != nil {
		t.Fatalf("UAPICall: %v", err)
	}
	if !res.OK() {
		t.Fatal("result not OK")
	}
	if res.Data.Email != "bob@example.com" {
		t.Fatalf("data = %+v", res.Data)
	}
	q := getReq().URL.Query()
	if q.Get("email") != "bob" || q.Get("domain") != "example.com" || q.Get("quota") != "250" {
		t.Fatalf("query args = %v", q)
	}
	if got := getReq().URL.Path; got != "/execute/Email/add_pop" {
		t.Fatalf("path = %s", got)
	}
}

func TestUAPICallWrappedFormat(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"apiversion":3,"func":"get_default_email_quota","module":"Email","result":{"data":{"quota":500},"errors":null,"messages":null,"status":1,"warnings":null}}`)
	})
	res, err := cpanel.UAPICall[map[string]int64](context.Background(), c, http.MethodGet, "Email", "get_default_email_quota", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Data["quota"] != 500 {
		t.Fatalf("data = %v", res.Data)
	}
	if res.APIVersion != 3 || res.Module != "Email" || res.Func != "get_default_email_quota" {
		t.Fatalf("envelope = %+v", res)
	}
}

func TestUAPICallFailure(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":null,"errors":["The password is too weak."],"messages":[],"metadata":{},"status":0,"warnings":[]}`)
	})
	res, err := cpanel.UAPICall[any](context.Background(), c, http.MethodGet, "Email", "add_pop", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *cpanel.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T", err)
	}
	if res == nil || res.OK() {
		t.Fatal("expected non-nil failing result")
	}
	if !strings.Contains(err.Error(), "too weak") {
		t.Fatalf("error = %v", err)
	}
	if len(res.ErrorStrings()) != 1 {
		t.Fatalf("errors = %v", res.ErrorStrings())
	}
}

func TestWHMCall(t *testing.T) {
	c, getReq := lastRequest(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"acct":[{"user":"bob","domain":"example.com"}]},"metadata":{"command":"listaccts","reason":"OK","result":1,"version":1}}`)
	})
	type acct struct {
		User   string `json:"user"`
		Domain string `json:"domain"`
	}
	res, err := cpanel.WHMCall[map[string][]acct](context.Background(), c, http.MethodGet, "listaccts", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatal("not OK")
	}
	if len(res.Data["acct"]) != 1 || res.Data["acct"][0].User != "bob" {
		t.Fatalf("data = %+v", res.Data)
	}
	if res.Metadata.Command != "listaccts" || res.Metadata.Reason != "OK" || res.Metadata.Result != 1 {
		t.Fatalf("metadata = %+v", res.Metadata)
	}
	if q := getReq().URL.Query(); q.Get("api.version") != "1" {
		t.Fatalf("missing api.version: %v", q)
	}
	if got := getReq().URL.Path; got != "/json-api/listaccts" {
		t.Fatalf("path = %s", got)
	}
}

func TestWHMCallFailure(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"metadata":{"command":"createacct","reason":"Sorry, that username is taken.","result":0,"version":1}}`)
	})
	res, err := cpanel.WHMCall[any](context.Background(), c, http.MethodGet, "createacct", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *cpanel.Error
	if !errors.As(err, &apiErr) || apiErr.Reason != "Sorry, that username is taken." {
		t.Fatalf("err = %v", err)
	}
	if res == nil || res.OK() {
		t.Fatalf("result = %+v", res)
	}
}

func TestHTTPError(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Access denied", http.StatusForbidden)
	})
	_, err := cpanel.UAPICall[any](context.Background(), c, http.MethodGet, "Email", "list_pops", nil)
	var apiErr *cpanel.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("type = %T", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", apiErr.StatusCode)
	}
	if apiErr.Op == "" {
		t.Fatal("op not annotated")
	}
}

func TestStructArgsEncoding(t *testing.T) {
	type args struct {
		Email    string      `cpanel:"email"`
		Quota    *int64      `cpanel:"quota,omitempty"`
		Domain   *string     `cpanel:"domain,omitempty"`
		Flags    []string    `cpanel:"flags,omitempty"`
		Skip     *bool       `cpanel:"skip_welcome,omitempty"`
		Missing  *string     `cpanel:"missing,omitempty"`
		Anything any         `cpanel:"anything,omitempty"`
		Extra    cpanel.Args `cpanel:"-"`
	}
	q := int64(250)
	d := "example.com"
	b := true
	got, err := cpanel.EncodeArgs(&args{
		Email: "bob", Quota: &q, Domain: &d, Skip: &b,
		Flags:    []string{"a", "b"},
		Anything: map[string]any{"nested": true},
		Extra:    cpanel.Args{}.Set("api.paginate.size", "10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{
		"email":             {"bob"},
		"quota":             {"250"},
		"domain":            {"example.com"},
		"skip_welcome":      {"1"},
		"flags":             {"a", "b"},
		"anything":          {`{"nested":true}`},
		"api.paginate.size": {"10"},
	}
	for k, vs := range want {
		if fmt.Sprint(got[k]) != fmt.Sprint(vs) {
			t.Fatalf("key %s = %v, want %v (all: %v)", k, got[k], vs, got)
		}
	}
	if got.Has("missing") {
		t.Fatalf("nil optional pointer encoded: %v", got)
	}
}

func TestEncodeArgsNilAndMaps(t *testing.T) {
	v, err := cpanel.EncodeArgs(nil)
	if err != nil || len(v) != 0 {
		t.Fatalf("nil encode = %v, %v", v, err)
	}
	v, err = cpanel.EncodeArgs(cpanel.Args{"a": "1"})
	if err != nil || v.Get("a") != "1" {
		t.Fatalf("map encode = %v, %v", v, err)
	}
}

func TestPOSTBodyEncoding(t *testing.T) {
	var body []byte
	c, getReq := lastRequest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		fmt.Fprint(w, `{"status":1,"data":{}}`)
	})
	if _, err := cpanel.UAPICall[any](context.Background(), c, http.MethodPost, "SSL", "upload_key",
		cpanel.Args{"key": "PEMDATA"}); err != nil {
		t.Fatal(err)
	}
	r := getReq()
	if r.Method != http.MethodPost {
		t.Fatalf("method = %s", r.Method)
	}
	if !strings.Contains(string(body), "key=PEMDATA") {
		t.Fatalf("POST body = %q", body)
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestAPI2(t *testing.T) {
	c, getReq := lastRequest(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"apiversion":2,"func":"listpops","module":"Email","cpanelresult":{"apiversion":2,"data":[{"email":"a@b.c"}],"error":"","event":{"result":1},"func":"listpops","module":"Email","preevent":{"result":1},"postevent":{"result":1}}}`)
	})
	res, err := c.API2(context.Background(), "bob", "Email", "listpops", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("not ok: %+v", res)
	}
	var pops []struct {
		Email string `json:"email"`
	}
	if err := res.DecodeData(&pops); err != nil || len(pops) != 1 || pops[0].Email != "a@b.c" {
		t.Fatalf("data decode: %v %v", pops, err)
	}
	q := getReq().URL.Query()
	if q.Get("cpanel_jsonapi_user") != "bob" || q.Get("cpanel_jsonapi_funcversion") != "2" ||
		q.Get("cpanel_jsonapi_module") != "Email" || q.Get("cpanel_jsonapi_func") != "listpops" {
		t.Fatalf("query = %v", q)
	}
}

func TestLooseString(t *testing.T) {
	var s struct {
		Errors []cpanel.LooseString `json:"errors"`
	}
	if err := json.Unmarshal([]byte(`{"errors":["plain",{"detail":"obj"},42,null]}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Errors[0] != "plain" {
		t.Fatalf("0: %q", s.Errors[0])
	}
	if string(s.Errors[1]) != `{"detail":"obj"}` {
		t.Fatalf("1: %q", s.Errors[1])
	}
	if string(s.Errors[2]) != `42` {
		t.Fatalf("2: %q", s.Errors[2])
	}
	if s.Errors[3] != "" {
		t.Fatalf("3: %q", s.Errors[3])
	}
}

func TestSessionURLBase(t *testing.T) {
	c, err := cpanel.NewClient("https://example.com:2083/cpsess12345", nil)
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"status":1,"data":{}}`)
	}))
	defer srv.Close()
	c2, err := cpanel.NewClient(srv.URL+"/cpsess999", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = c
	if _, err := cpanel.UAPICall[any](context.Background(), c2, http.MethodGet, "Email", "list_pops", nil); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/cpsess999/execute/Email/list_pops" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestPtrHelpers(t *testing.T) {
	if *cpanel.String("x") != "x" || *cpanel.Int(int64(4)) != 4 || *cpanel.Bool(true) != true || *cpanel.Float(1.5) != 1.5 {
		t.Fatal("ptr helpers")
	}
	if a := cpanel.CombineArgs(cpanel.Args{"a": "1"}, cpanel.Args{"a": "2", "b": "3"}); a["a"] != "2" || a["b"] != "3" {
		t.Fatalf("combine: %v", a)
	}
}
