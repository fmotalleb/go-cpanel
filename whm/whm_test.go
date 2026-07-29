package whm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	cpanel "github.com/fmotalleb/go-cpanel"
	"github.com/fmotalleb/go-cpanel/whm"
)

func server(t *testing.T, h http.HandlerFunc) *cpanel.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := cpanel.NewClient(srv.URL, cpanel.WHMTokenAuth("root", "WHMTOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListAccts(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json-api/listaccts" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "whm root:WHMTOKEN" {
			t.Errorf("auth = %q", got)
		}
		fmt.Fprint(w, `{"data":{"acct":[{"user":"bob","domain":"example.com","diskused":"12M","backup":1}]},"metadata":{"command":"listaccts","reason":"OK","result":1,"version":1}}`)
	})
	res, err := whm.New(c).ListAccts(context.Background(), &whm.ListAcctsArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.Data.Acct) != 1 || res.Data.Acct[0].User != "bob" || res.Data.Acct[0].Diskused != "12M" {
		t.Fatalf("result = %+v", res)
	}
}

func TestCreateAcctFailure(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"metadata":{"command":"createacct","reason":"That username is taken.","result":0,"version":1}}`)
	})
	res, err := whm.New(c).CreateAcct(context.Background(), &whm.CreateAcctArgs{
		Username: "bob", Domain: cpanel.String("example.com"), Password: cpanel.String("hunter2"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if res == nil || res.OK() {
		t.Fatalf("result = %+v", res)
	}
	if res.Metadata.Reason != "That username is taken." {
		t.Fatalf("reason = %q", res.Metadata.Reason)
	}
}
