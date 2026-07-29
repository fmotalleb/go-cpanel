package cpanel

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent is sent with every request unless overridden by
// WithUserAgent.
const DefaultUserAgent = "go-cpanel (+https://github.com/fmotalleb/go-cpanel)"

// Client talks to a single cPanel or WHM server.
//
// Create one with NewClient. A Client is safe for concurrent use by multiple
// goroutines.
type Client struct {
	baseURL   *url.URL
	auth      Authenticator
	http      *http.Client
	userAgent string
	headers   http.Header
}

// Option customises a Client. See the With* functions.
type Option func(*Client)

// WithHTTPClient sets the *http.Client used for requests.
//
// Use this to configure timeouts, TLS settings (for example
// InsecureSkipVerify for servers with self-signed certificates), proxies,
// transports and so on.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithInsecureSkipVerify accepts invalid/self-signed TLS certificates.
//
// cPanel & WHM servers frequently ship with self-signed certificates on
// ports 2083/2087, which makes this option useful for direct connections.
// Prefer installing/trusting the server certificate whenever possible.
func cloneDefaultTransport() *http.Transport {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok || tr == nil {
		return &http.Transport{}
	}
	return tr.Clone()
}

func WithInsecureSkipVerify(skip bool) Option {
	return func(c *Client) {
		tr, ok := c.http.Transport.(*http.Transport)
		if !ok || tr == nil {
			tr = cloneDefaultTransport()
		} else {
			tr = tr.Clone()
		}
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.InsecureSkipVerify = skip
		c.http.Transport = tr
	}
}

// WithTimeout sets a simple per-request timeout.
//
// For finer-grained control build your own *http.Client and pass it via
// WithHTTPClient.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithHeader sets an extra header on every outgoing request.
func WithHeader(key, value string) Option {
	return func(c *Client) { c.headers.Set(key, value) }
}

// NewClient creates a client for the cPanel or WHM server at rawBaseURL.
//
// rawBaseURL must include the scheme, host and port, and may optionally
// include a session path (for example
// "https://cpanel.example.com:2083/cpsess1234567890" for session-based
// authentication). Auth may be nil when the base URL itself authenticates
// the request (session URLs); for server-to-server integrations pass one of
// the authenticators from this package.
//
//	c, err := cpanel.NewClient("https://cpanel.example.com:2083",
//	    cpanel.CPanelTokenAuth(user, token),
//	)
func NewClient(rawBaseURL string, auth Authenticator, opts ...Option) (*Client, error) {
	u, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("cpanel: invalid base URL %q: %w", rawBaseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("cpanel: invalid base URL %q: scheme and host are required", rawBaseURL)
	}
	c := &Client{
		baseURL:   u,
		auth:      auth,
		http:      &http.Client{},
		userAgent: DefaultUserAgent,
		headers:   http.Header{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// BaseURL returns the server's base URL.
func (c *Client) BaseURL() *url.URL { cp := *c.baseURL; return &cp }

// endpoint joins the base URL with the given API path elements.
func (c *Client) endpoint(elem ...string) string {
	u := *c.baseURL
	parts := []string{strings.TrimSuffix(u.EscapedPath(), "/")}
	for _, e := range elem {
		parts = append(parts, url.PathEscape(e))
	}
	u.Path = strings.Join(parts, "/")
	u.RawPath = ""
	return u.String()
}

// doRaw performs the HTTP request and returns the raw response body.
//
// GET and HEAD requests receive their arguments via the query string; all
// other methods receive them as an application/x-www-form-urlencoded body,
// which cPanel & WHM accept for every function.
func (c *Client) doRaw(ctx context.Context, method, fullURL string, args any) ([]byte, error) {
	values, err := EncodeArgs(args)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodDelete {
		if sep := "&"; strings.Contains(fullURL, "?") {
			fullURL += sep + values.Encode()
		} else if len(values) > 0 {
			fullURL += "?" + values.Encode()
		}
	} else {
		body = bytes.NewBufferString(values.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("cpanel: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if c.auth != nil {
		c.auth.Authenticate(req)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpanel: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("cpanel: reading response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, newHTTPError(resp.StatusCode, data)
	}
	return data, nil
}

// marshalJSON exists so the encoding machinery has a single place to convert
// nested argument structures.
func marshalJSON(v any) ([]byte, error) { return json.Marshal(v) }
