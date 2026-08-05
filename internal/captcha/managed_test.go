package captcha

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew: known providers build a verifier, "none"/"" disable it (nil, nil), a
// managed provider with no secret and an unknown provider both fail closed.
func TestNew(t *testing.T) {
	for _, p := range []string{ProviderTurnstile, ProviderHCaptcha, ProviderRecaptcha} {
		v, err := New(p, "secret")
		require.NoError(t, err, p)
		require.NotNil(t, v, p)
	}

	for _, disabled := range []string{ProviderNone, ""} {
		v, err := New(disabled, "")
		require.NoError(t, err)
		assert.Nil(t, v, "disabled provider %q yields no verifier", disabled)
	}

	_, err := New(ProviderTurnstile, "")
	assert.Error(t, err, "a managed provider requires a secret")

	_, err = New("nope", "secret")
	assert.Error(t, err, "an unknown provider fails closed")
}

// TestManagedVerifierPassesAndSendsForm: a success=true response is a pass, and the
// verifier POSTs the secret, the response token, and the remote IP as form fields.
func TestManagedVerifierPassesAndSendsForm(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success": true}`)
	}))
	defer srv.Close()

	v := &managedVerifier{endpoint: srv.URL, secret: "shh", client: srv.Client()}
	require.NoError(t, v.Verify(context.Background(), "tok-123", "203.0.113.7"))
	assert.Equal(t, "shh", got.Get("secret"))
	assert.Equal(t, "tok-123", got.Get("response"))
	assert.Equal(t, "203.0.113.7", got.Get("remoteip"))
}

// TestManagedVerifierRefuses: a success=false body, a non-200 status, and an
// undecodable body all refuse (fail-closed) — the caller sees an error either way.
func TestManagedVerifierRefuses(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"success=false": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"success": false, "error-codes": ["invalid-input-response"]}`)
		},
		"non-200": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
		"garbage-body": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `not json`)
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			v := &managedVerifier{endpoint: srv.URL, secret: "shh", client: srv.Client()}
			assert.Error(t, v.Verify(context.Background(), "tok", ""))
		})
	}
}

// TestManagedVerifierOmitsEmptyRemoteIP: an empty remote IP is not sent as a form
// field (some providers reject an empty remoteip).
func TestManagedVerifierOmitsEmptyRemoteIP(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `{"success": true}`)
	}))
	defer srv.Close()

	v := &managedVerifier{endpoint: srv.URL, secret: "shh", client: srv.Client()}
	require.NoError(t, v.Verify(context.Background(), "tok", ""))
	_, present := got["remoteip"]
	assert.False(t, present, "empty remote IP must be omitted")
}

// TestNoopVerifierAccepts: the disabled default accepts any token.
func TestNoopVerifierAccepts(t *testing.T) {
	assert.NoError(t, NoopVerifier{}.Verify(context.Background(), "", ""))
	assert.NoError(t, NoopVerifier{}.Verify(context.Background(), "anything", "1.2.3.4"))
}
