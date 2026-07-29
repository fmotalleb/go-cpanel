// Command api2-demo demonstrates calling the deprecated cPanel API 2,
// including calls proxied through WHM (acting as a specific cPanel user).
//
// Configuration via environment variables:
//
//	WHM_BASE_URL    e.g. https://whm.example.com:2087
//	WHM_USER        usually root
//	WHM_API_TOKEN   a WHM API token
//	CPANEL_USER     a cPanel account on the server to act on
package main

import (
	"context"
	"log"
	"os"
	"time"

	cpanel "github.com/fmotalleb/go-cpanel"
)

func main() {
	baseURL := os.Getenv("WHM_BASE_URL")
	user := os.Getenv("WHM_USER")
	token := os.Getenv("WHM_API_TOKEN")
	cpUser := os.Getenv("CPANEL_USER")

	if baseURL == "" || user == "" || token == "" || cpUser == "" {
		log.Fatal("set WHM_BASE_URL, WHM_USER, WHM_API_TOKEN and CPANEL_USER")
	}

	client, err := cpanel.NewClient(baseURL, cpanel.WHMTokenAuth(user, token),
		cpanel.WithInsecureSkipVerify(true),
		cpanel.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Calls the deprecated Email::listpops cPanel API 2 function on behalf of
	// cpUser through the WHM proxy (port 2087 /json-api/cpanel).
	res, err := client.WHMAPI2(ctx, cpUser, "Email", "listpops", nil)
	if err != nil {
		log.Fatalf("api2 Email::listpops: %v", err)
	}

	var accounts []struct {
		Email string `json:"email"`
		Login string `json:"login"`
	}
	if err := res.DecodeData(&accounts); err != nil {
		log.Fatalf("decode: %v", err)
	}
	log.Printf("%s has %d email accounts (API 2)", cpUser, len(accounts))
	for _, a := range accounts {
		log.Printf("  - %s", a.Email)
	}
}
