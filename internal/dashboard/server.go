// Package dashboard serves kupra's dashboard: its page, and a backend that signs people in and passes their requests to the Kubernetes API as them (ADR 7).
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"path"

	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

// csrfHeader must be on every request that changes something. A cross-site form can't set it, and a cross-site script can't without the dashboard allowing it through CORS, which it doesn't.
const csrfHeader = "X-Kupra-CSRF"

// Options configures the dashboard.
type Options struct {
	// PublicURL is where people reach the dashboard, such as https://previews.example.com.
	PublicURL string
	// IssuerURL, ClientID, ClientSecret and Scopes configure signing in with OIDC. The dashboard needs a confidential client: its cookies are encrypted with keys derived from ClientSecret.
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// Config reaches the Kubernetes API server. Its credentials are the dashboard's own, and are only used to find namespaces with apps.
	Config *rest.Config
	// Page is the built page to serve. It's the one embedded in the binary when nil.
	Page fs.FS
	// HTTPClient talks to the identity provider. It's http.DefaultClient when nil.
	HTTPClient *http.Client
}

// New returns the dashboard's handler.
func New(ctx context.Context, opts Options) (http.Handler, error) {
	public, err := url.Parse(opts.PublicURL)
	if err != nil || public.Host == "" {
		return nil, errors.New("public URL must be absolute")
	}
	auth, err := newAuthenticator(ctx, opts, public.Scheme == "https")
	if err != nil {
		return nil, err
	}
	own, err := dynamic.NewForConfig(opts.Config)
	if err != nil {
		return nil, err
	}
	// Requests as users carry their token and none of the dashboard's credentials.
	anonymous := rest.AnonymousClientConfig(opts.Config)
	transport, err := rest.TransportFor(anonymous)
	if err != nil {
		return nil, err
	}
	server, err := url.Parse(anonymous.Host)
	if err != nil {
		return nil, err
	}
	server.Path = path.Join(server.Path, anonymous.APIPath)

	h := &handler{auth: auth, anonymous: anonymous}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /auth/login", auth.login)
	mux.HandleFunc("GET /auth/callback", auth.callback)
	mux.HandleFunc("POST /auth/logout", auth.logout)
	mux.HandleFunc("GET /api/session", h.session)
	mux.Handle("GET /api/watch", &watchHandler{auth: auth, anonymous: anonymous, own: own})
	mux.Handle(kubePrefix+"/", newKubeProxy(auth, server, transport))
	page := opts.Page
	if page == nil {
		page = builtPage()
	}
	mux.Handle("/", spa(page))
	return securityHeaders(csrf(mux)), nil
}

type handler struct {
	auth      *authenticator
	anonymous *rest.Config
}

// session says who the API server takes the signed-in user to be.
func (h *handler) session(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.token(w, r)
	if err != nil {
		authError(w, r, err)
		return
	}
	cfg := rest.CopyConfig(h.anonymous)
	cfg.BearerToken = token
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		httpError(w, r, http.StatusInternalServerError, err)
		return
	}
	review, err := client.AuthenticationV1().SelfSubjectReviews().Create(r.Context(), &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		kubeError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"username": review.Status.UserInfo.Username,
		"groups":   review.Status.UserInfo.Groups,
	})
}

// csrf refuses requests that change something unless they carry csrfHeader.
func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) == "" {
			http.Error(w, csrfHeader+" header is required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// authError sends 401 for a request without a usable session, so the page sends the browser to sign in.
func authError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errSignedOut) {
		http.Error(w, "signed out", http.StatusUnauthorized)
		return
	}
	httpError(w, r, http.StatusBadGateway, err)
}

// kubeError passes on the status of a failed request to the API server.
func kubeError(w http.ResponseWriter, r *http.Request, err error) {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		http.Error(w, status.Status().Message, int(status.Status().Code))
		return
	}
	httpError(w, r, http.StatusBadGateway, err)
}

func httpError(w http.ResponseWriter, r *http.Request, code int, err error) {
	ctrl.LoggerFrom(r.Context()).Error(err, "request failed", "path", r.URL.Path)
	http.Error(w, http.StatusText(code), code)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
