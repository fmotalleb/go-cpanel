package cpanel

import (
	"net/http"
)

// Authenticator applies authentication to an outgoing HTTP request.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type Authenticator interface {
	// Authenticate sets the authentication credentials on req.
	Authenticate(req *http.Request)
}

// AuthenticatorFunc adapts a plain function to the Authenticator interface.
type AuthenticatorFunc func(req *http.Request)

// Authenticate implements Authenticator.
func (f AuthenticatorFunc) Authenticate(req *http.Request) { f(req) }

// BasicAuth authenticates with a username and password.
//
// It is accepted by both the cPanel (2083) and WHM (2087) APIs and is the
// least secure of the supported mechanisms; prefer an API token when
// possible. See: https://api.docs.cpanel.net/whm/introduction/#authentication
func BasicAuth(user, password string) Authenticator {
	return AuthenticatorFunc(func(req *http.Request) {
		req.SetBasicAuth(user, password)
	})
}

// CPanelTokenAuth authenticates against cPanel (port 2083) with a cPanel API
// token, as issued by UAPI's Tokens::create_full_access / Tokens::create or
// the "Manage API Tokens" cPanel interface.
//
// The Authorization header takes the form:
//
//	Authorization: cpanel <user>:<token>
//
// See: https://api.docs.cpanel.net/cpanel/tokens/
func CPanelTokenAuth(user, token string) Authenticator {
	return AuthenticatorFunc(func(req *http.Request) {
		req.Header.Set("Authorization", "cpanel "+user+":"+token)
	})
}

// WHMTokenAuth authenticates against WHM (port 2087) with a WHM API token,
// as issued by WHM's "Manage API Tokens" interface (or the api_token_create
// function).
//
// The Authorization header takes the form:
//
//	Authorization: whm <user>:<token>
//
// See: https://api.docs.cpanel.net/whm/tokens/
func WHMTokenAuth(user, token string) Authenticator {
	return AuthenticatorFunc(func(req *http.Request) {
		req.Header.Set("Authorization", "whm "+user+":"+token)
	})
}

// AccessHashAuth authenticates against WHM (port 2087) with a legacy WHM
// access hash (remote access key). New integrations should use
// WHMTokenAuth instead, as access hashes are deprecated.
func AccessHashAuth(user, accessHash string) Authenticator {
	return AuthenticatorFunc(func(req *http.Request) {
		req.Header.Set("Authorization", "whm "+user+":"+accessHash)
	})
}
