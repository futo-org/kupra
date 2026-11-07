package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"k8s.io/client-go/rest"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// provider is a fake OIDC provider. It signs anyone in as alice, with ID tokens that last ttl.
type provider struct {
	*httptest.Server
	key *rsa.PrivateKey
	ttl time.Duration

	mu        sync.Mutex
	nonces    map[string]string // by code
	refreshed int
	// unavailable is how many more refreshes fail with 503 Service Unavailable.
	unavailable int
	// refreshToken is the one refresh token the provider accepts.
	refreshToken string
	revoked      []string
}

func newProvider(t *testing.T) *provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{key: key, ttl: time.Hour, nonces: map[string]string{}}
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
		code := randomString()
		p.mu.Lock()
		p.nonces[code] = q.Get("nonce")
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
			var ok bool
			if nonce, ok = p.nonces[r.Form.Get("code")]; !ok || r.Form.Get("code_verifier") == "" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
		case "refresh_token":
			if p.unavailable > 0 {
				p.unavailable--
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			// Refresh tokens rotate: each is accepted once.
			if r.Form.Get("refresh_token") != p.refreshToken {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			p.refreshed++
		}
		p.refreshToken = fmt.Sprintf("refresh-%d", p.refreshed)
		writeJSON(w, map[string]any{
			"access_token":  "access",
			"token_type":    "Bearer",
			"refresh_token": p.refreshToken,
			"id_token":      p.idToken(t, nonce),
		})
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		p.revoked = append(p.revoked, r.Form.Get("token"))
		p.mu.Unlock()
	})
	p.Server = httptest.NewServer(mux)
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
	f := &fixture{provider: newProvider(t), api: newAPIServer(t)}
	static := t.TempDir()
	if err := os.WriteFile(filepath.Join(static, "index.html"), []byte("<p>kupra</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
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
		CookieSecret: strings.Repeat("s", 32),
		Config:       &rest.Config{Host: f.api.URL, BearerToken: "dashboard-token"},
		StaticDir:    static,
	})
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	f.browser = &http.Client{Jar: jar}
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

	code, body = f.do(t, http.MethodGet, "/api/app-namespaces", nil)
	if code != http.StatusOK || body != `{"namespaces":["immich"]}`+"\n" {
		t.Errorf("app namespaces are %d %s, want immich", code, body)
	}
	got = f.api.last()
	if got.URL.Query().Get("labelSelector") != kuprav1alpha1.LabelAppNamespace || got.Header.Get("Authorization") != "Bearer dashboard-token" {
		t.Errorf("namespaces were listed with selector %q and Authorization %q, want %s with the dashboard's own token", got.URL.Query().Get("labelSelector"), got.Header.Get("Authorization"), kuprav1alpha1.LabelAppNamespace)
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
		{http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces%2fimmich/previews", http.StatusForbidden},
		{http.MethodGet, "/api/kube/apis/kupra.futo.tech/v1alpha1x/previews", http.StatusForbidden},
		{http.MethodGet, "/api/kube/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", http.StatusForbidden},
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

func TestSealer(t *testing.T) {
	s, err := newSealer(strings.Repeat("k", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	long := session{IDToken: strings.Repeat("x", 3*chunkSize)}
	rec := httptest.NewRecorder()
	if err := s.set(rec, httptest.NewRequest(http.MethodGet, "/", nil), sessionCookie, "/", time.Hour, long); err != nil {
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
	var got session
	if !s.get(req, sessionCookie, &got) || got.IDToken != long.IDToken {
		t.Error("long session doesn't open again")
	}
	if s.get(req, loginCookie, &got) {
		t.Error("session opens as another cookie")
	}

	// A shorter value expires the chunks the longer one left.
	rec = httptest.NewRecorder()
	if err := s.set(rec, req, sessionCookie, "/", time.Hour, session{IDToken: "short"}); err != nil {
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
	if s.get(tampered, sessionCookie, &got) {
		t.Error("tampered session opens")
	}
	other, _ := newSealer(strings.Repeat("o", 32), true)
	if other.get(req, sessionCookie, &got) {
		t.Error("session opens with another secret")
	}
}

// A refresh that fails for a while keeps the session, so the next request can refresh it. One the provider refuses ends it.
func TestRefreshFailures(t *testing.T) {
	f := newFixture(t)
	f.provider.ttl = refreshMargin / 2
	f.signIn(t, "/")
	const path = "/api/kube/apis/kupra.futo.tech/v1alpha1/previews"

	f.provider.unavailable = 1
	if code, _ := f.do(t, http.MethodGet, path, nil); code != http.StatusBadGateway {
		t.Errorf("request while the provider is unavailable is %d, want 502", code)
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
