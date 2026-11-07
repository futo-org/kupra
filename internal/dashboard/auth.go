package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	sessionCookie = "kupra_session"
	loginCookie   = "kupra_login"
	// sessionLifetime is how long the browser keeps the session's cookies. The tokens inside expire on their own terms.
	sessionLifetime = 30 * 24 * time.Hour
	loginLifetime   = 10 * time.Minute
	// refreshMargin refreshes an ID token this long before it expires, so it doesn't expire on its way to the API server.
	refreshMargin  = 30 * time.Second
	refreshTimeout = 30 * time.Second
)

// revokeTimeout bounds how long signing out waits for the provider to revoke the refresh token.
var revokeTimeout = 10 * time.Second

// errSignedOut means a request has no usable session.
var errSignedOut = errors.New("signed out")

// authenticator signs people in with an OIDC provider, and keeps their tokens in sealed cookies.
type authenticator struct {
	oauth2     oauth2.Config
	verifier   *oidc.IDTokenVerifier
	revocation string
	sealer     *sealer
	// refreshes shares a refresh between the requests that carry the same refresh token, as providers that rotate refresh tokens only accept each once.
	refreshes refreshes
	// client is used for every request to the identity provider.
	client *http.Client
}

func newAuthenticator(ctx context.Context, opts Options, s *sealer) (*authenticator, error) {
	ctx = oidc.ClientContext(ctx, opts.HTTPClient)
	provider, err := oidc.NewProvider(ctx, opts.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discovering the OIDC provider: %w", err)
	}
	var endpoints struct {
		Revocation string `json:"revocation_endpoint"`
	}
	if err := provider.Claims(&endpoints); err != nil {
		return nil, fmt.Errorf("reading the OIDC provider's metadata: %w", err)
	}
	return &authenticator{
		oauth2: oauth2.Config{
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimSuffix(opts.PublicURL, "/") + "/auth/callback",
			Scopes:       opts.Scopes,
		},
		verifier:   provider.Verifier(&oidc.Config{ClientID: opts.ClientID}),
		revocation: endpoints.Revocation,
		sealer:     s,
		client:     opts.HTTPClient,
	}, nil
}

func (a *authenticator) context(ctx context.Context) context.Context {
	if a.client == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, a.client)
}

// login sends the browser to the identity provider.
func (a *authenticator) login(w http.ResponseWriter, r *http.Request) {
	l := login{
		State:    randomString(),
		Nonce:    randomString(),
		Verifier: oauth2.GenerateVerifier(),
		Redirect: localRedirect(r.URL.Query().Get("redirect")),
	}
	if err := a.sealer.set(w, r, loginCookie, "/auth/", loginLifetime, l); err != nil {
		httpError(w, r, http.StatusInternalServerError, err)
		return
	}
	http.Redirect(w, r, a.oauth2.AuthCodeURL(l.State, oidc.Nonce(l.Nonce), oauth2.S256ChallengeOption(l.Verifier)), http.StatusFound)
}

// callback finishes signing in when the identity provider sends the browser back.
func (a *authenticator) callback(w http.ResponseWriter, r *http.Request) {
	var l login
	if !a.sealer.get(r, loginCookie, &l) {
		http.Error(w, "sign-in expired or was started in another browser; sign in again", http.StatusBadRequest)
		return
	}
	a.sealer.clear(w, r, loginCookie, "/auth/")
	q := r.URL.Query()
	if q.Get("state") != l.State {
		http.Error(w, "sign-in state doesn't match; sign in again", http.StatusBadRequest)
		return
	}
	if e := q.Get("error"); e != "" {
		http.Error(w, "identity provider refused sign-in: "+e+" "+q.Get("error_description"), http.StatusForbidden)
		return
	}
	ctx := a.context(r.Context())
	token, err := a.oauth2.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(l.Verifier))
	if err != nil {
		httpError(w, r, http.StatusBadGateway, fmt.Errorf("exchanging the authorization code: %w", err))
		return
	}
	s, err := a.session(ctx, token, l.Nonce)
	if err != nil {
		httpError(w, r, http.StatusBadGateway, err)
		return
	}
	if err := a.sealer.set(w, r, sessionCookie, "/", sessionLifetime, s); err != nil {
		httpError(w, r, http.StatusInternalServerError, err)
		return
	}
	http.Redirect(w, r, l.Redirect, http.StatusFound)
}

// session verifies the ID token in a token response. An empty nonce skips checking it, as refreshed ID tokens needn't carry one.
func (a *authenticator) session(ctx context.Context, token *oauth2.Token, nonce string) (*session, error) {
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("identity provider returned no ID token")
	}
	idToken, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("verifying the ID token: %w", err)
	}
	if nonce != "" && idToken.Nonce != nonce {
		return nil, errors.New("ID token's nonce doesn't match")
	}
	return &session{IDToken: raw, RefreshToken: token.RefreshToken, Expiry: idToken.Expiry}, nil
}

// token returns the request's ID token, refreshing it if it's about to expire.
func (a *authenticator) token(w http.ResponseWriter, r *http.Request) (string, error) {
	var s session
	if !a.sealer.get(r, sessionCookie, &s) {
		return "", errSignedOut
	}
	if time.Until(s.Expiry) > refreshMargin {
		return s.IDToken, nil
	}
	if s.RefreshToken == "" {
		return "", errSignedOut
	}
	refreshed, err := a.refreshes.do(s.RefreshToken, func() (*session, error) {
		// Other requests wait on this refresh, so it goes on if this request is cancelled.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), refreshTimeout)
		defer cancel()
		return a.refresh(a.context(ctx), s.RefreshToken)
	})
	var refused *oauth2.RetrieveError
	if errors.As(err, &refused) && refused.ErrorCode == "invalid_grant" {
		// The provider refused the refresh token, so the session is over. Other errors, like being rate limited, may pass.
		ctrl.LoggerFrom(r.Context()).Info("provider refused to refresh a session", "error", err.Error())
		a.sealer.clear(w, r, sessionCookie, "/")
		return "", errSignedOut
	}
	if err != nil {
		// Otherwise the session is kept, so a later request can try again.
		return "", fmt.Errorf("refreshing the session: %w", err)
	}
	if err := a.sealer.set(w, r, sessionCookie, "/", sessionLifetime, refreshed); err != nil {
		return "", err
	}
	return refreshed.IDToken, nil
}

// refresh exchanges a refresh token for a new session.
func (a *authenticator) refresh(ctx context.Context, refreshToken string) (*session, error) {
	token, err := a.oauth2.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return nil, err
	}
	refreshed, err := a.session(ctx, token, "")
	if err != nil {
		return nil, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = refreshToken
	}
	return refreshed, nil
}

// logout revokes the session's refresh token, if the provider supports it, and clears the session's cookies.
func (a *authenticator) logout(w http.ResponseWriter, r *http.Request) {
	var s session
	if a.sealer.get(r, sessionCookie, &s) && s.RefreshToken != "" && a.revocation != "" {
		// Signing out doesn't wait long for the provider: the session's cookies are cleared either way.
		ctx, cancel := context.WithTimeout(r.Context(), revokeTimeout)
		defer cancel()
		if err := a.revoke(ctx, s.RefreshToken); err != nil {
			ctrl.LoggerFrom(r.Context()).Info("revoking a refresh token failed", "error", err.Error())
		}
	}
	a.sealer.clear(w, r, sessionCookie, "/")
	w.WriteHeader(http.StatusNoContent)
}

func (a *authenticator) revoke(ctx context.Context, refreshToken string) error {
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.revocation, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(a.oauth2.ClientID), url.QueryEscape(a.oauth2.ClientSecret))
	client := a.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revocation endpoint returned %s", resp.Status)
	}
	return nil
}

// localRedirect keeps a redirect to a path on the dashboard itself, so signing in can't send people elsewhere.
func localRedirect(target string) string {
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.ContainsAny(target, `\`) {
		return "/"
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return target
}

func randomString() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
