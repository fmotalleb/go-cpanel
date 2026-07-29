// Package cpanel provides a complete Go client for the cPanel & WHM APIs:
//
//   - cPanel UAPI (https://api.docs.cpanel.net/specifications/cpanel.openapi);
//     642 functions across 97 modules, fully generated with typed arguments and
//     typed responses (see the sub-package uapi).
//   - WHM API 1 (https://api.docs.cpanel.net/specifications/whm.openapi);
//     625 functions across 49 categories, fully generated with typed arguments
//     and typed responses (see the sub-package whm).
//   - cPanel API 2 (deprecated); a generic executor is provided via
//     Client.API2 and Client.WHMAPI2, both directly on a cPanel server and
//     through the WHM proxy.
//
// The generated code is derived from the official OpenAPI 3 documents that
// power cPanel's own developer portal (they apply to cPanel & WHM version 138).
//
// # Quick start
//
//	import (
//	    "context"
//	    "github.com/fmotalleb/go-cpanel"
//	    "github.com/fmotalleb/go-cpanel/uapi"
//	    "github.com/fmotalleb/go-cpanel/whm"
//	)
//
//	// cPanel (port 2083), authenticated with an API token:
//	c, _ := cpanel.NewClient("https://cpanel.example.com:2083",
//	    cpanel.CPanelTokenAuth("user", "CPANELAPITOKEN"))
//	pops, err := uapi.New(c).Email().ListPops(context.Background(), nil)
//
//	// WHM (port 2087), authenticated as root with an API token:
//	server, _ := cpanel.NewClient("https://whm.example.com:2087",
//	    cpanel.WHMTokenAuth("root", "WHMAPITOKEN"))
//	accts, err := whm.New(server).ListAccts(context.Background(), nil)
//
// # Authentication
//
// The package supports every credential scheme accepted by cPanel & WHM:
// username+password (BasicAuth), cPanel API tokens (CPanelTokenAuth),
// WHM API tokens (WHMTokenAuth) and the legacy WHM access hash
// (AccessHashAuth).
//
// Everything in this package uses only the Go standard library.
package cpanel
