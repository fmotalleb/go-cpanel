package uapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	cpanel "github.com/fmotalleb/go-cpanel"
	"github.com/fmotalleb/go-cpanel/uapi"
)

func server(t *testing.T, h http.HandlerFunc) *cpanel.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := cpanel.NewClient(srv.URL, cpanel.CPanelTokenAuth("user", "TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEmailListPops(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute/Email/list_pops" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "cpanel user:TOKEN" {
			t.Errorf("auth = %q", got)
		}
		fmt.Fprint(w, `{"data":[{"email":"bob@example.com","login":"bob@example.com","suspended_incoming":0,"suspended_login":0}],"errors":[],"messages":[],"metadata":{},"status":1,"warnings":[]}`)
	})
	res, err := uapi.New(c).Email().ListPops(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.Data) != 1 || res.Data[0].Email != "bob@example.com" {
		t.Fatalf("result = %+v", res)
	}
}

func TestEmailAddPopArgsEncoding(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("email") != "bob" || q.Get("domain") != "example.com" || q.Get("quota") != "250" {
			t.Errorf("query = %v", q)
		}
		fmt.Fprint(w, `{"data":"bob@example.com","errors":[],"messages":[],"metadata":{},"status":1,"warnings":[]}`)
	})
	res, err := uapi.New(c).Email().AddPop(context.Background(), &uapi.EmailAddPopArgs{
		Email:    "bob",
		Domain:   cpanel.String("example.com"),
		Password: "hunter2",
		Quota:    cpanel.Int(int64(250)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Data != "bob@example.com" {
		t.Fatalf("data = %v", res.Data)
	}
}

func TestModuleClientCoverage(t *testing.T) {
	// A sampling of module accessors across the 97 generated modules.
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":1,"data":{}}`)
	})
	_ = c
	uc := uapi.New(c)
	if uc.Core() != c {
		t.Fatal("Core")
	}
	_ = uc.Email()
	_ = uc.Mysql()
	_ = uc.SSL()
	_ = uc.FTP()
	_ = uc.DNS()
	_ = uc.Backup()
	_ = uc.Tokens()
	_ = uc.CPGreyList()
	_ = uc.WordPressSite()
}
