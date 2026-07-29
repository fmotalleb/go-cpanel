// Command uapi-demo demonstrates calling cPanel UAPI functions with the
// generated typed wrappers.
//
// Configuration via environment variables:
//
//	CPANEL_BASE_URL   e.g. https://cpanel.example.com:2083
//	CPANEL_USER       the cPanel account name
//	CPANEL_API_TOKEN  a cPanel API token (otherwise CPANEL_PASSWORD is used)
//	CPANEL_PASSWORD   the account password (fallback)
//	CPANEL_DOMAIN     a domain on the account (for the demo calls)
package main

import (
	"context"
	"log"
	"os"
	"time"

	cpanel "github.com/fmotalleb/go-cpanel"
	"github.com/fmotalleb/go-cpanel/uapi"
)

func main() {
	baseURL := os.Getenv("CPANEL_BASE_URL")
	user := os.Getenv("CPANEL_USER")
	token := os.Getenv("CPANEL_API_TOKEN")
	password := os.Getenv("CPANEL_PASSWORD")
	domain := os.Getenv("CPANEL_DOMAIN")

	if baseURL == "" || user == "" || (token == "" && password == "") {
		log.Fatal("set CPANEL_BASE_URL, CPANEL_USER and CPANEL_API_TOKEN (or CPANEL_PASSWORD)")
	}

	var auth cpanel.Authenticator
	if token != "" {
		auth = cpanel.CPanelTokenAuth(user, token)
	} else {
		auth = cpanel.BasicAuth(user, password)
	}

	client, err := cpanel.NewClient(baseURL, auth,
		// Most cPanel servers use self-signed certs on 2083; remove in
		// production if your server has a valid certificate.
		cpanel.WithInsecureSkipVerify(true),
		cpanel.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uc := uapi.New(client)

	// List email accounts on the account.
	pops, err := uc.Email().ListPops(ctx, nil)
	if err != nil {
		log.Fatalf("Email::list_pops: %v", err)
	}
	log.Printf("email accounts: %d", len(pops.Data))
	for _, acct := range pops.Data {
		log.Printf("  - %s (login: %s)", acct.Email, acct.Login)
	}

	// List the domains on the account.
	domains, err := uc.DomainInfo().ListDomains(ctx, nil)
	if err != nil {
		log.Fatalf("DomainInfo::list_domains: %v", err)
	}
	if domains.Data.MainDomain != nil {
		log.Printf("main domain: %s", *domains.Data.MainDomain)
	}

	// Create an email account (skipped unless CPANEL_DOMAIN is set);
	// delete it again right away so the demo stays idempotent.
	if domain != "" {
		add, err := uc.Email().AddPop(ctx, &uapi.EmailAddPopArgs{
			Email:    "go-cpanel-demo",
			Domain:   cpanel.String(domain),
			Password: "demo-password-" + time.Now().Format("150405"),
		})
		if err != nil {
			log.Printf("Email::add_pop: %v", err)
		} else {
			log.Printf("created mailbox: %s", add.Data)
			del, err := uc.Email().DeletePop(ctx, &uapi.EmailDeletePopArgs{
				Email: add.Data,
			})
			if err != nil {
				log.Printf("Email::delete_pop: %v", err)
			} else {
				log.Printf("deleted mailbox again: %v", del.OK())
			}
		}
	}
}
