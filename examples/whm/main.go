// Command whm-demo demonstrates calling WHM API 1 functions with the
// generated typed wrappers.
//
// Configuration via environment variables:
//
//	WHM_BASE_URL    e.g. https://whm.example.com:2087
//	WHM_USER        usually root (or a reseller)
//	WHM_API_TOKEN   a WHM API token (otherwise WHM_PASSWORD is used)
//	WHM_PASSWORD    the WHM password (fallback)
package main

import (
	"context"
	"log"
	"os"
	"time"

	cpanel "github.com/fmotalleb/go-cpanel"
	"github.com/fmotalleb/go-cpanel/whm"
)

func main() {
	baseURL := os.Getenv("WHM_BASE_URL")
	user := os.Getenv("WHM_USER")
	token := os.Getenv("WHM_API_TOKEN")
	password := os.Getenv("WHM_PASSWORD")

	if baseURL == "" || user == "" || (token == "" && password == "") {
		log.Fatal("set WHM_BASE_URL, WHM_USER and WHM_API_TOKEN (or WHM_PASSWORD)")
	}

	var auth cpanel.Authenticator
	if token != "" {
		auth = cpanel.WHMTokenAuth(user, token)
	} else {
		auth = cpanel.BasicAuth(user, password)
	}

	client, err := cpanel.NewClient(baseURL, auth,
		cpanel.WithInsecureSkipVerify(true),
		cpanel.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	server := whm.New(client)

	// Server overview.
	hostname, err := server.Gethostname(ctx)
	if err != nil {
		log.Fatalf("gethostname: %v", err)
	}
	log.Printf("hostname: %s", hostname.Data.Hostname)

	// List hosting accounts (limit to 5 using WHM pagination meta arguments).
	accts, err := server.ListAccts(ctx, &whm.ListAcctsArgs{
		Extra: cpanel.Args{}.Set("api.paginate", "1").Set("api.paginate.size", "5"),
	})
	if err != nil {
		log.Fatalf("listaccts: %v", err)
	}
	log.Printf("accounts (page size 5): %d", len(accts.Data.Acct))
	for _, acct := range accts.Data.Acct {
		log.Printf("  - %s (%s) plan=%s disk=%s", acct.User, acct.Domain, acct.Plan, acct.Diskused)
	}

	// List packages.
	pkgs, err := server.Listpkgs(ctx, nil)
	if err != nil {
		log.Fatalf("listpkgs: %v", err)
	}
	for _, pkg := range pkgs.Data.Pkg {
		log.Printf("  package %s (quota=%v)", pkg.Name, pkg.Quota)
	}
}
