// Command dashboard serves kupra's dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/futo-org/kupra/internal/dashboard"
)

func main() {
	var addr, publicURL, issuerURL, clientID, scopes, staticDir string
	flag.StringVar(&addr, "bind-address", ":8080", "The address the dashboard serves on.")
	flag.StringVar(&publicURL, "public-url", "", "The URL people reach the dashboard at.")
	flag.StringVar(&issuerURL, "oidc-issuer-url", os.Getenv("OIDC_ISSUER_URL"), "The OIDC provider's issuer URL. Defaults to $OIDC_ISSUER_URL.")
	flag.StringVar(&clientID, "oidc-client-id", os.Getenv("OIDC_CLIENT_ID"), "The dashboard's OIDC client ID. Defaults to $OIDC_CLIENT_ID.")
	flag.StringVar(&scopes, "oidc-scopes", "openid,profile,email,groups,offline_access", "The scopes to request, comma-separated.")
	flag.StringVar(&staticDir, "static-dir", "", "The directory holding the built page.")
	logOpts := zap.Options{}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	log := ctrl.Log.WithName("dashboard")
	ctx := ctrl.SetupSignalHandler()

	handler, err := dashboard.New(ctx, dashboard.Options{
		PublicURL:    publicURL,
		IssuerURL:    issuerURL,
		ClientID:     clientID,
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		Scopes:       strings.Split(scopes, ","),
		Config:       ctrl.GetConfigOrDie(),
		StaticDir:    staticDir,
	})
	if err != nil {
		log.Error(err, "unable to set up the dashboard")
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Info("serving", "address", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "serving failed")
		os.Exit(1)
	}
}
