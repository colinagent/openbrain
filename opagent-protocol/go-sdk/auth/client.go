// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

// OAuthHandler is an interface for handling OAuth flows.
//
// If a transport wishes to support OAuth 2 authorization, it should support
// being configured with an OAuthHandler. It should call the handler's
// TokenSource method whenever it sends an HTTP request to set the
// Authorization header. If a request fails with a 401 or 403, it should call
// Authorize, and if that returns nil, it should retry the request. It should
// not call Authorize after the second failure. See
// [github.com/modelcontextprotocol/go-sdk/mcp.StreamableClientTransport]
// for an example.
type OAuthHandler interface {
	// TokenSource returns a token source to be used for outgoing requests.
	// Returned token source might be nil. In that case, the transport will not
	// add any authorization headers to the request.
	TokenSource(context.Context) (oauth2.TokenSource, error)

	// Authorize is called when an HTTP request results in an error that may
	// be addressed by the authorization flow (currently 401 Unauthorized and 403 Forbidden).
	// It is responsible for performing the OAuth flow to obtain an access token.
	// The arguments are the request that failed and the response that was received for it.
	// The headers of the request are available, but the body will have already been consumed
	// when Authorize is called.
	// If the returned error is nil, TokenSource is expected to return a non-nil token source.
	// After a successful call to Authorize, the HTTP request will be retried by the transport.
	// The function is responsible for closing the response body.
	Authorize(context.Context, *http.Request, *http.Response) error
}

// HTTPTransport is an [http.RoundTripper] that follows the MCP
// OAuth protocol when it encounters a 401 Unauthorized response.
type HTTPTransport struct {
	handler OAuthHandler
	mu      sync.Mutex // protects opts.Base
	opts    HTTPTransportOptions
}

// NewHTTPTransport returns a new [*HTTPTransport].
// The handler is invoked when an HTTP request results in a 401 Unauthorized status.
// It is called only once per transport. Once a TokenSource is obtained, it is used
// for the lifetime of the transport; subsequent 401s are not processed.
func NewHTTPTransport(handler OAuthHandler, opts *HTTPTransportOptions) (*HTTPTransport, error) {
	if handler == nil {
		return nil, errors.New("handler cannot be nil")
	}
	t := &HTTPTransport{
		handler: handler,
	}
	if opts != nil {
		t.opts = *opts
	}
	if t.opts.Base == nil {
		t.opts.Base = http.DefaultTransport
	}
	return t, nil
}

// HTTPTransportOptions are options to [NewHTTPTransport].
type HTTPTransportOptions struct {
	// Base is the [http.RoundTripper] to use.
	// If nil, [http.DefaultTransport] is used.
	Base http.RoundTripper
}

func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	base := t.opts.Base
	t.mu.Unlock()

	var (
		// If haveBody is set, the request has a nontrivial body, and we need avoid
		// reading (or closing) it multiple times. In that case, bodyBytes is its
		// content.
		haveBody  bool
		bodyBytes []byte
	)
	if req.Body != nil && req.Body != http.NoBody {
		// if we're setting Body, we must mutate first.
		req = req.Clone(req.Context())
		haveBody = true
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		// Now that we've read the request body, http.RoundTripper requires that we
		// close it.
		req.Body.Close() // ignore error
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	if _, ok := base.(*oauth2.Transport); ok {
		// We failed to authorize even with a token source; give up.
		return resp, nil
	}

	resp.Body.Close()
	// Try to authorize.
	t.mu.Lock()
	defer t.mu.Unlock()
	// If we don't have a token source, get one by following the OAuth flow.
	// (We may have obtained one while t.mu was not held above.)
	// TODO: We hold the lock for the entire OAuth flow. This could be a long
	// time. Is there a better way?
	if _, ok := t.opts.Base.(*oauth2.Transport); !ok {
		if err := t.handler.Authorize(req.Context(), req, resp); err != nil {
			return nil, err
		}
		ts, err := t.handler.TokenSource(req.Context())
		if err != nil {
			return nil, err
		}
		t.opts.Base = &oauth2.Transport{Base: t.opts.Base, Source: ts}
	}

	// If we don't have a body, the request is reusable, though it will be cloned
	// by the base. However, if we've had to read the body, we must clone.
	if haveBody {
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return t.opts.Base.RoundTrip(req)
}
