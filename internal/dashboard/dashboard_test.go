package dashboard

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-jose/go-jose/v4"
	"k8s.io/client-go/rest"
)

// provider is a fake OIDC provider. It signs anyone in as alice, with ID tokens that last ttl.
type provider struct {
	*httptest.Server
	key *rsa.PrivateKey
	ttl time.Duration

	mu        sync.Mutex
	codes     map[string]authorization
	refreshed int
	// refreshWithoutIDToken has refreshes answer without an ID token.
	refreshWithoutIDToken bool
	// hangRevocation makes revocation never answer.
	hangRevocation bool
	// failures are the statuses the next refreshes fail with.
	failures []int
	// keepRefreshToken has refreshes return the same refresh token.
	keepRefreshToken bool
	// refreshToken is the one refresh token the provider accepts.
	refreshToken string
	revoked      []string
}

// authorization is what the provider keeps of an authorization request until its code is exchanged.
type authorization struct {
	nonce, challenge, method string
}

func newProvider(t *testing.T) *provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{key: key, ttl: time.Hour, codes: map[string]authorization{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                p.URL,
			"authorization_endpoint":                p.URL + "/authorize",
			"token_endpoint":                        p.URL + "/token",
			"jwks_uri":                              p.URL + "/keys",
			"revocation_endpoint":                   p.URL + "/revoke",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k", Algorithm: "RS256", Use: "sig"}}})
	})
	// The authorization endpoint signs alice in straight away, and sends her back with a code.
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := rand.Text()
		p.mu.Lock()
		p.codes[code] = authorization{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), method: q.Get("code_challenge_method")}
		p.mu.Unlock()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		defer p.mu.Unlock()
		nonce := ""
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			auth, ok := p.codes[r.Form.Get("code")]
			// PKCE: the verifier has to hash to the challenge the authorization request carried.
			verifier := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || auth.method != "S256" || base64.RawURLEncoding.EncodeToString(verifier[:]) != auth.challenge {
				invalidGrant(w)
				return
			}
			nonce = auth.nonce
		case "refresh_token":
			if len(p.failures) > 0 {
				code := p.failures[0]
				p.failures = p.failures[1:]
				http.Error(w, http.StatusText(code), code)
				return
			}
			// Refresh tokens rotate, unless keepRefreshToken is set: each is accepted once.
			if r.Form.Get("refresh_token") != p.refreshToken {
				invalidGrant(w)
				return
			}
			p.refreshed++
		}
		if !p.keepRefreshToken || p.refreshToken == "" {
			p.refreshToken = fmt.Sprintf("refresh-%d", p.refreshed)
		}
		response := map[string]any{"access_token": "access", "token_type": "Bearer", "refresh_token": p.refreshToken}
		if r.Form.Get("grant_type") != "refresh_token" || !p.refreshWithoutIDToken {
			response["id_token"] = p.idToken(t, nonce)
		}
		writeJSON(w, response)
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		if p.hangRevocation {
			// The server only notices the client going away once the body is read.
			_ = r.ParseForm()
			<-r.Context().Done()
			return
		}
		_ = r.ParseForm()
		p.mu.Lock()
		p.revoked = append(p.revoked, r.Form.Get("token"))
		p.mu.Unlock()
	})
	// TLS, as the API server only trusts issuers over https.
	p.Server = httptest.NewTLSServer(mux)
	t.Cleanup(p.Close)
	return p
}

func (p *provider) idToken(t *testing.T, nonce string) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: p.key, KeyID: "k"}}, nil)
	if err != nil {
		t.Error(err)
		return ""
	}
	claims := map[string]any{"iss": p.URL, "aud": "kupra", "sub": "alice", "exp": time.Now().Add(p.ttl).Unix(), "iat": time.Now().Unix()}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	payload, _ := json.Marshal(claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Error(err)
		return ""
	}
	s, _ := jws.CompactSerialize()
	return s
}

// apiServer is a fake Kubernetes API server that records the requests it gets.
type apiServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newAPIServer(t *testing.T) *apiServer {
	a := &apiServer{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.requests = append(a.requests, r.Clone(context.Background()))
		a.mu.Unlock()
		switch r.URL.Path {
		case "/apis/authentication.k8s.io/v1/selfsubjectreviews":
			writeJSON(w, map[string]any{
				"apiVersion": "authentication.k8s.io/v1", "kind": "SelfSubjectReview",
				"status": map[string]any{"userInfo": map[string]any{"username": "oidc:alice", "groups": []string{"oidc:developers"}}},
			})
		case "/api/v1/namespaces":
			writeJSON(w, map[string]any{
				"apiVersion": "v1", "kind": "NamespaceList",
				"items": []any{map[string]any{"metadata": map[string]any{"name": "immich"}}},
			})
		default:
			writeJSON(w, map[string]any{"kind": "PreviewList", "items": []any{}})
		}
	}))
	t.Cleanup(a.Close)
	return a
}

func (a *apiServer) last() *http.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.requests) == 0 {
		return nil
	}
	return a.requests[len(a.requests)-1]
}

type fixture struct {
	provider  *provider
	api       *apiServer
	dashboard *httptest.Server
	browser   *http.Client
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	api := newAPIServer(t)
	f := newFixtureWith(t, newProvider(t), &rest.Config{Host: api.URL, BearerToken: "dashboard-token"})
	f.api = api
	return f
}

// newFixtureWith serves a dashboard that signs in with p, and reaches the API server with cfg as its own account.
func newFixtureWith(t *testing.T, p *provider, cfg *rest.Config) *fixture {
	t.Helper()
	f := &fixture{provider: p}
	var handler http.Handler
	f.dashboard = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(f.dashboard.Close)
	var err error
	handler, err = New(context.Background(), Options{
		PublicURL:    f.dashboard.URL,
		IssuerURL:    f.provider.URL,
		ClientID:     "kupra",
		ClientSecret: "secret",
		Scopes:       []string{"openid", "offline_access"},
		Config:       cfg,
		HTTPClient:   p.Client(),
		Page:         fstest.MapFS{"index.html": {Data: []byte("<p>kupra</p>")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	f.browser = &http.Client{Jar: jar, Transport: p.Client().Transport}
	return f
}

func (f *fixture) do(t *testing.T, method, path string, header http.Header) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, f.dashboard.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := f.browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func (f *fixture) signIn(t *testing.T, redirect string) string {
	t.Helper()
	resp, err := f.browser.Get(f.dashboard.URL + "/auth/login?redirect=" + url.QueryEscape(redirect))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signing in ended with %s, want the page", resp.Status)
	}
	return resp.Request.URL.RequestURI()
}

func TestSignInAndProxy(t *testing.T) {
	f := newFixture(t)
	if code, _ := f.do(t, http.MethodGet, "/api/session", nil); code != http.StatusUnauthorized {
		t.Errorf("session before signing in is %d, want 401", code)
	}

	if landed := f.signIn(t, "/apps/immich?tab=previews"); landed != "/apps/immich?tab=previews" {
		t.Errorf("signing in landed on %s, want the page it started from", landed)
	}
	code, body := f.do(t, http.MethodGet, "/api/session", nil)
	if code != http.StatusOK || !strings.Contains(body, `"username":"oidc:alice"`) {
		t.Errorf("session is %d %s, want oidc:alice", code, body)
	}
	if auth := f.api.last().Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ey") {
		t.Errorf("session was looked up with %q, want alice's ID token", auth)
	}

	code, _ = f.do(t, http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces/immich/previews?watch=true", http.Header{
		"Impersonate-User": {"admin"},
		"Authorization":    {"Bearer forged"},
	})
	if code != http.StatusOK {
		t.Fatalf("listing previews is %d, want 200", code)
	}
	got := f.api.last()
	if got.URL.Path != "/apis/kupra.futo.tech/v1alpha1/namespaces/immich/previews" || got.URL.RawQuery != "watch=true" {
		t.Errorf("API server got %s, want the previews path with watch=true", got.URL)
	}
	if auth := got.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ey") {
		t.Errorf("API server got Authorization %q, want alice's ID token", auth)
	}
	for _, h := range []string{"Cookie", "Impersonate-User"} {
		if v := got.Header.Get(h); v != "" {
			t.Errorf("API server got %s %q, want none", h, v)
		}
	}

	code, _ = f.do(t, http.MethodPost, "/auth/logout", nil)
	if code != http.StatusForbidden {
		t.Errorf("logging out without %s is %d, want 403", csrfHeader, code)
	}
	code, _ = f.do(t, http.MethodPost, "/auth/logout", http.Header{csrfHeader: {"1"}})
	if code != http.StatusNoContent {
		t.Errorf("logging out is %d, want 204", code)
	}
	if len(f.provider.revoked) != 1 || f.provider.revoked[0] != "refresh-0" {
		t.Errorf("provider revoked %v, want the refresh token", f.provider.revoked)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/session", nil); code != http.StatusUnauthorized {
		t.Errorf("session after logging out is %d, want 401", code)
	}
}

func TestProxyRefusesOtherAPIs(t *testing.T) {
	f := newFixture(t)
	f.signIn(t, "/")
	// Redirects are what the test checks, not followed.
	f.browser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	before := len(f.api.requests)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/kube/api/v1/namespaces/immich/secrets", http.StatusForbidden},
		// The server's mux redirects dot segments to the cleaned path, which is refused like any other.
		{http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/../../../api/v1/secrets", http.StatusTemporaryRedirect},
		{http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/%2e%2e/%2e%2e/%2e%2e/api/v1/secrets", http.StatusForbidden},
		{http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1x/previews", http.StatusForbidden},
		{http.MethodPost, "/api/kube/apis/authorization.k8s.io/v1/subjectaccessreviews", http.StatusForbidden},
	} {
		req, _ := http.NewRequest(tc.method, f.dashboard.URL, nil)
		// An opaque URL is sent as is, without the client cleaning or escaping it.
		req.URL.Opaque = tc.path
		req.Header.Set(csrfHeader, "1")
		resp, err := f.browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s is %s, want %d", tc.method, tc.path, resp.Status, tc.want)
		}
	}
	if after := len(f.api.requests); after != before {
		t.Errorf("API server got %d requests, want none", after-before)
	}
}

func TestRefresh(t *testing.T) {
	f := newFixture(t)
	// Tokens that expire within the refresh margin are refreshed on every use.
	f.provider.ttl = refreshMargin / 2
	f.signIn(t, "/")

	// Requests sent together with the same cookies share one refresh, as the provider accepts each refresh token once.
	var wg sync.WaitGroup
	codes := make([]int, 10)
	for i := range codes {
		wg.Go(func() {
			req, _ := http.NewRequest(http.MethodGet, f.dashboard.URL+"/api/kube/apis/kupra.futo.tech/v1alpha1/previews", nil)
			for _, c := range f.browser.Jar.Cookies(req.URL) {
				req.AddCookie(c)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_ = resp.Body.Close()
			codes[i] = resp.StatusCode
		})
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d is %d, want 200", i, code)
		}
	}
	if f.provider.refreshed != 1 {
		t.Errorf("provider refreshed %d times, want once", f.provider.refreshed)
	}

	// The browser still has the old cookies, so its next request gets the same refresh's result, and the new cookies. The one after carries the new refresh token, and refreshes again.
	if code, _ := f.do(t, http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/previews", nil); code != http.StatusOK {
		t.Fatalf("listing previews is %d, want 200", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/previews", nil); code != http.StatusOK {
		t.Fatalf("listing previews again is %d, want 200", code)
	}
	if f.provider.refreshed != 2 {
		t.Errorf("provider refreshed %d times, want twice", f.provider.refreshed)
	}
}

func TestSPA(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/", "/apps/immich"} {
		if code, body := f.do(t, http.MethodGet, path, nil); code != http.StatusOK || body != "<p>kupra</p>" {
			t.Errorf("%s is %d %q, want the page", path, code, body)
		}
	}
	if code, _ := f.do(t, http.MethodGet, "/api/unknown", nil); code != http.StatusNotFound {
		t.Errorf("unknown API path is %d, want 404", code)
	}
}

func TestLocalRedirect(t *testing.T) {
	for target, want := range map[string]string{
		"/apps/immich?x=1":     "/apps/immich?x=1",
		"":                     "/",
		"https://evil.example": "/",
		"//evil.example/path":  "/",
		`/\evil.example`:       "/",
		"javascript:alert(1)":  "/",
		"/ok#fragment":         "/ok#fragment",
	} {
		if got := localRedirect(target); got != want {
			t.Errorf("redirect %q becomes %q, want %q", target, got, want)
		}
	}
}

// A refresh that fails for a while keeps the session, so the next request can refresh it. One the provider refuses ends it.
func TestRefreshFailures(t *testing.T) {
	f := newFixture(t)
	f.provider.ttl = refreshMargin / 2
	f.signIn(t, "/")
	const path = "/api/kube/apis/kupra.futo.tech/v1alpha1/previews"

	f.provider.failures = []int{http.StatusServiceUnavailable, http.StatusTooManyRequests}
	for _, failure := range f.provider.failures {
		if code, _ := f.do(t, http.MethodGet, path, nil); code != http.StatusBadGateway {
			t.Errorf("request while the provider answers %d is %d, want 502", failure, code)
		}
	}
	if code, _ := f.do(t, http.MethodGet, path, nil); code != http.StatusOK {
		t.Errorf("request once the provider is back is %d, want 200", code)
	}

	f.provider.mu.Lock()
	f.provider.refreshToken = "revoked"
	f.provider.mu.Unlock()
	if code, _ := f.do(t, http.MethodGet, path, nil); code != http.StatusUnauthorized {
		t.Errorf("request with a refused refresh token is %d, want 401", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/session", nil); code != http.StatusUnauthorized {
		t.Errorf("session after a refused refresh is %d, want 401", code)
	}
}

// invalidGrant refuses a grant the way OAuth 2.0 says to, with a JSON error.
func invalidGrant(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
}

// Signing out clears the session even when the provider doesn't answer the revocation.
func TestLogoutWhenRevocationHangs(t *testing.T) {
	timeout := revokeTimeout
	revokeTimeout = 100 * time.Millisecond
	t.Cleanup(func() { revokeTimeout = timeout })
	f := newFixture(t)
	f.signIn(t, "/")
	f.provider.hangRevocation = true
	if code, _ := f.do(t, http.MethodPost, "/auth/logout", http.Header{csrfHeader: {"1"}}); code != http.StatusNoContent {
		t.Errorf("logging out is %d, want 204", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/session", nil); code != http.StatusUnauthorized {
		t.Errorf("session after logging out is %d, want 401", code)
	}
}

// With a provider that keeps refresh tokens, a finished refresh isn't handed out once its ID token has expired.
func TestRefreshKeptToken(t *testing.T) {
	f := newFixture(t)
	f.provider.keepRefreshToken = true
	f.provider.ttl = 3 * time.Second
	f.signIn(t, "/")
	const path = "/api/kube/apis/kupra.futo.tech/v1alpha1/previews"
	for i := range 2 {
		if i > 0 {
			// ID tokens expire to the second.
			time.Sleep(4 * time.Second)
		}
		if code, _ := f.do(t, http.MethodGet, path, nil); code != http.StatusOK {
			t.Fatalf("request %d is %d, want 200", i, code)
		}
	}
	if f.provider.refreshed != 2 {
		t.Errorf("provider refreshed %d times, want twice", f.provider.refreshed)
	}
}

func TestSessions(t *testing.T) {
	hashKey, blockKey, err := cookieKeys("client-secret", "sessions")
	if err != nil {
		t.Fatal(err)
	}
	s := newSessions(hashKey, blockKey, true)
	long := session{IDToken: strings.Repeat("x", 3*chunkSize), Expiry: time.Now().Add(time.Hour).Truncate(time.Second)}
	rec := httptest.NewRecorder()
	if err := s.set(rec, httptest.NewRequest(http.MethodGet, "/", nil), long); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) < 3 {
		t.Fatalf("long session is in %d cookies, want it split over several", len(cookies))
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		if len(c.String()) > 4096 {
			t.Errorf("cookie %s is %d bytes, want at most 4096", c.Name, len(c.String()))
		}
		req.AddCookie(c)
	}
	if got, ok := s.get(req); !ok || got.IDToken != long.IDToken {
		t.Error("long session doesn't open again")
	}

	// A shorter session expires the chunks the longer one left.
	rec = httptest.NewRecorder()
	if err := s.set(rec, req, session{IDToken: "short"}); err != nil {
		t.Fatal(err)
	}
	expired := 0
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			expired++
		}
	}
	if expired != len(cookies)-1 {
		t.Errorf("shorter session expired %d cookies, want %d", expired, len(cookies)-1)
	}

	tampered := httptest.NewRequest(http.MethodGet, "/", nil)
	for i, c := range cookies {
		if i == 0 {
			c.Value = "A" + c.Value[1:]
		}
		tampered.AddCookie(c)
	}
	if _, ok := s.get(tampered); ok {
		t.Error("tampered session opens")
	}
	otherHash, otherBlock, _ := cookieKeys("rotated-secret", "sessions")
	if _, ok := newSessions(otherHash, otherBlock, true).get(req); ok {
		t.Error("session opens after the client secret changed")
	}
}

// A dashboard built without its page says how to build it, rather than serving an empty directory.
func TestSPAWithoutPage(t *testing.T) {
	rec := httptest.NewRecorder()
	spa(fstest.MapFS{".gitkeep": {}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apps/immich", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "mise run web:build") {
		t.Errorf("page without a build is %d %q, want 503 saying to run mise run web:build", rec.Code, rec.Body.String())
	}
}

// The fake provider holds sign-in to PKCE, so the sign-in tests would catch a dashboard that sends the wrong verifier.
func TestProviderChecksPKCE(t *testing.T) {
	p := newProvider(t)
	client := p.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	verifier := rand.Text() + rand.Text()
	challenge := sha256.Sum256([]byte(verifier))
	exchange := func(v string) int {
		t.Helper()
		resp, err := client.Get(p.URL + "/authorize?" + url.Values{
			"redirect_uri":          {"https://dashboard.example.com/auth/callback"},
			"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
			"code_challenge_method": {"S256"},
		}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		location, _ := url.Parse(resp.Header.Get("Location"))
		resp, err = client.PostForm(p.URL+"/token", url.Values{
			"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "code_verifier": {v},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := exchange("wrong" + verifier); code != http.StatusBadRequest {
		t.Errorf("exchanging a code with the wrong verifier is %d, want 400", code)
	}
	if code := exchange(verifier); code != http.StatusOK {
		t.Errorf("exchanging a code with its verifier is %d, want 200", code)
	}
}

// A refresh that succeeds without an ID token, which the API server needs, ends the session, whether or not the refresh token rotated.
func TestRefreshWithoutIDToken(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep refresh token %t", keep), func(t *testing.T) {
			f := newFixture(t)
			f.provider.ttl = refreshMargin / 2
			f.provider.keepRefreshToken = keep
			f.signIn(t, "/")
			f.provider.mu.Lock()
			f.provider.refreshWithoutIDToken = true
			f.provider.mu.Unlock()
			if code, _ := f.do(t, http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/previews", nil); code != http.StatusUnauthorized {
				t.Errorf("request is %d, want 401", code)
			}
			if code, _ := f.do(t, http.MethodGet, "/api/session", nil); code != http.StatusUnauthorized {
				t.Errorf("session afterwards is %d, want 401", code)
			}
		})
	}
}

// An idle stream sends keepalive comments, so proxies don't close it and make the page list everything again.
func TestWatchKeepsAlive(t *testing.T) {
	interval := heartbeatInterval
	heartbeatInterval = 50 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = interval })
	f := newFixture(t)
	f.signIn(t, "/")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.dashboard.URL+"/api/watch?c=app-namespaces", nil)
	resp, err := f.browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	lines := bufio.NewScanner(resp.Body)
	for lines.Scan() {
		if lines.Text() == ": keepalive" {
			return
		}
	}
	t.Errorf("stream ended without a keepalive: %v", lines.Err())
}
