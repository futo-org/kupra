package dashboard

import (
	"context"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// sessionLifetime is how long the browser keeps the session's cookies. The tokens inside expire on their own terms.
	sessionLifetime = 30 * 24 * time.Hour
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
	party    rp.RelyingParty
	sessions *sessions
	// refreshes shares a refresh between the requests that carry the same refresh token, as providers that rotate refresh tokens only accept each once.
	refreshes refreshes
}

// cookieKeys derives the keys that authenticate and encrypt the dashboard's cookies from the OIDC client's secret, so there's no other secret to keep. Rotating the client secret signs everyone out.
func cookieKeys(clientSecret, purpose string) (hashKey, blockKey []byte, err error) {
	if clientSecret == "" {
		return nil, nil, errors.New("OIDC client secret is required")
	}
	keys, err := hkdf.Key(sha256.New, []byte(clientSecret), nil, "kupra dashboard "+purpose, 64)
	if err != nil {
		return nil, nil, err
	}
	return keys[:32], keys[32:], nil
}

func newAuthenticator(ctx context.Context, opts Options, secure bool) (*authenticator, error) {
	hashKey, blockKey, err := cookieKeys(opts.ClientSecret, "sign-in")
	if err != nil {
		return nil, err
	}
	cookieOpts := []httphelper.CookieHandlerOpt{httphelper.WithPath("/auth/"), httphelper.WithSameSite(http.SameSiteLaxMode)}
	if !secure {
		cookieOpts = append(cookieOpts, httphelper.WithUnsecure())
	}
	rpOpts := []rp.Option{rp.WithPKCE(httphelper.NewCookieHandler(hashKey, blockKey, cookieOpts...))}
	if opts.HTTPClient != nil {
		rpOpts = append(rpOpts, rp.WithHTTPClient(opts.HTTPClient))
	}
	party, err := rp.NewRelyingPartyOIDC(ctx, opts.IssuerURL, opts.ClientID, opts.ClientSecret,
		strings.TrimSuffix(opts.PublicURL, "/")+"/auth/callback", opts.Scopes, rpOpts...)
	if err != nil {
		return nil, fmt.Errorf("setting up the OIDC client: %w", err)
	}
	hashKey, blockKey, err = cookieKeys(opts.ClientSecret, "sessions")
	if err != nil {
		return nil, err
	}
	return &authenticator{party: party, sessions: newSessions(hashKey, blockKey, secure)}, nil
}

// login sends the browser to the identity provider. The state carries where to return to, and the library checks it against a cookie.
func (a *authenticator) login(w http.ResponseWriter, r *http.Request) {
	state := rand.Text() + " " + localRedirect(r.URL.Query().Get("redirect"))
	rp.AuthURLHandler(func() string { return state }, a.party)(w, r)
}

// callback finishes signing in when the identity provider sends the browser back.
func (a *authenticator) callback(w http.ResponseWriter, r *http.Request) {
	rp.CodeExchangeHandler(func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, _ rp.RelyingParty) {
		s := session{IDToken: tokens.IDToken, RefreshToken: tokens.RefreshToken, Expiry: tokens.IDTokenClaims.GetExpiration()}
		if err := a.sessions.set(w, r, s); err != nil {
			httpError(w, r, http.StatusInternalServerError, err)
			return
		}
		_, redirect, _ := strings.Cut(state, " ")
		http.Redirect(w, r, localRedirect(redirect), http.StatusFound)
	}, a.party)(w, r)
}

// token returns the request's ID token, refreshing it if it's about to expire.
func (a *authenticator) token(w http.ResponseWriter, r *http.Request) (string, error) {
	s, ok := a.sessions.get(r)
	if !ok {
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
		return a.refresh(ctx, s.RefreshToken)
	})
	if errors.Is(err, oidc.ErrInvalidGrant()) {
		// The provider refused the refresh token, so the session is over. Other errors, like being rate limited, may pass.
		ctrl.LoggerFrom(r.Context()).Info("provider refused to refresh a session", "error", err.Error())
		a.sessions.clear(w, r)
		return "", errSignedOut
	}
	if err != nil {
		// Otherwise the session is kept, so a later request can try again.
		return "", fmt.Errorf("refreshing the session: %w", err)
	}
	if err := a.sessions.set(w, r, *refreshed); err != nil {
		return "", err
	}
	return refreshed.IDToken, nil
}

// refresh exchanges a refresh token for a new session.
func (a *authenticator) refresh(ctx context.Context, refreshToken string) (*session, error) {
	tokens, err := rp.RefreshTokens[*oidc.IDTokenClaims](ctx, a.party, refreshToken, "", "")
	if err != nil {
		return nil, err
	}
	if tokens.IDToken == "" {
		return nil, errors.New("identity provider returned no ID token")
	}
	s := &session{IDToken: tokens.IDToken, RefreshToken: tokens.RefreshToken, Expiry: tokens.IDTokenClaims.GetExpiration()}
	if s.RefreshToken == "" {
		s.RefreshToken = refreshToken
	}
	return s, nil
}

// logout revokes the session's refresh token, if the provider supports it, and clears the session's cookies.
func (a *authenticator) logout(w http.ResponseWriter, r *http.Request) {
	if s, ok := a.sessions.get(r); ok && s.RefreshToken != "" && a.party.GetRevokeEndpoint() != "" {
		// Signing out doesn't wait long for the provider: the session's cookies are cleared either way.
		ctx, cancel := context.WithTimeout(r.Context(), revokeTimeout)
		defer cancel()
		if err := rp.RevokeToken(ctx, a.party, s.RefreshToken, "refresh_token"); err != nil {
			ctrl.LoggerFrom(r.Context()).Info("revoking a refresh token failed", "error", err.Error())
		}
	}
	a.sessions.clear(w, r)
	w.WriteHeader(http.StatusNoContent)
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
