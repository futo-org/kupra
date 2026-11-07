package dashboard

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// kubePrefix is where the page reaches the Kubernetes API.
const kubePrefix = "/api/kube"

// allowedKube decides whether the page may make a request to the Kubernetes API: anything on kupra's resources, and access reviews to find out what it may do.
func allowedKube(method, path string) bool {
	for _, segment := range strings.Split(path, "/")[1:] {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	if path == "/apis/kupra.futo.tech/v1alpha1" || strings.HasPrefix(path, "/apis/kupra.futo.tech/v1alpha1/") {
		return true
	}
	return method == http.MethodPost && path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews"
}

// kubeProxy passes the page's requests to the Kubernetes API server, as the signed-in user.
type kubeProxy struct {
	auth  *authenticator
	proxy *httputil.ReverseProxy
}

func newKubeProxy(auth *authenticator, server *url.URL, transport http.RoundTripper) *kubeProxy {
	return &kubeProxy{
		auth: auth,
		proxy: &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme = server.Scheme
				pr.Out.URL.Host = server.Host
				pr.Out.Host = server.Host
				pr.Out.URL.Path = strings.TrimSuffix(server.Path, "/") + strings.TrimPrefix(pr.In.URL.Path, kubePrefix)
				pr.Out.URL.RawPath = ""
			},
			Transport: transport,
			// Watches stream, so responses are flushed as they come.
			FlushInterval: -1,
		},
	}
}

func (p *kubeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, kubePrefix)
	if r.URL.RawPath != "" || !allowedKube(r.Method, path) {
		http.Error(w, "the dashboard doesn't pass on "+r.Method+" "+path, http.StatusForbidden)
		return
	}
	token, err := p.auth.token(w, r)
	if err != nil {
		authError(w, r, err)
		return
	}
	out := r.Clone(r.Context())
	// Only what the API needs is passed on. Cookies hold the user's tokens, and headers like Impersonate-User aren't the page's to send.
	out.Header = http.Header{}
	for _, key := range []string{"Accept", "Content-Type"} {
		if v := r.Header.Values(key); len(v) > 0 {
			out.Header[key] = v
		}
	}
	out.Header.Set("Authorization", "Bearer "+token)
	p.proxy.ServeHTTP(w, out)
}
