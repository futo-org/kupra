package dashboard

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"

	kubeproxy "k8s.io/kubectl/pkg/proxy"
)

// kubePrefix is where the page reaches the Kubernetes API.
const kubePrefix = "/api/kube"

// kubeFilter decides what the page may ask of the Kubernetes API, the way `kubectl proxy` does: kupra's resources, and access reviews to find out what it may do with them. Paths with dot or empty segments are refused, so they can't step outside these.
var kubeFilter = &kubeproxy.FilterServer{
	AcceptPaths: []*regexp.Regexp{
		regexp.MustCompile(`^/apis/kupra\.futo\.tech/v1alpha1(/|$)`),
		regexp.MustCompile(`^/apis/authorization\.k8s\.io/v1/selfsubjectaccessreviews$`),
	},
	RejectPaths: []*regexp.Regexp{regexp.MustCompile(`(^|/)\.{1,2}(/|$)|//`)},
	AcceptHosts: []*regexp.Regexp{regexp.MustCompile(`.*`)},
}

// newKubeProxy passes the page's requests to the Kubernetes API server, as the signed-in user.
func newKubeProxy(auth *authenticator, server *url.URL, transport http.RoundTripper) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(server)
			// Only what the API needs is passed on. Cookies hold the user's tokens, and headers like Impersonate-User aren't the page's to send.
			header := http.Header{}
			for _, key := range []string{"Accept", "Content-Type", "Authorization"} {
				if v := pr.Out.Header.Values(key); len(v) > 0 {
					header[key] = v
				}
			}
			pr.Out.Header = header
		},
		Transport: transport,
		// Watches stream, so responses are flushed as they come.
		FlushInterval: -1,
	}
	authorized := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.token(w, r)
		if err != nil {
			authError(w, r, err)
			return
		}
		r.Header.Set("Authorization", "Bearer "+token)
		proxy.ServeHTTP(w, r)
	})
	return http.StripPrefix(kubePrefix, kubeFilter.HandlerFor(authorized))
}
