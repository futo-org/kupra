// Command dashboard serves kupra's dashboard.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
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
	var addr, publicURL, issuerURL, clientID, caFile, scopes string
	flag.StringVar(&addr, "bind-address", ":8080", "The address the dashboard serves on.")
	flag.StringVar(&publicURL, "public-url", "", "The URL people reach the dashboard at.")
	flag.StringVar(&issuerURL, "oidc-issuer-url", os.Getenv("OIDC_ISSUER_URL"), "The OIDC provider's issuer URL. Defaults to $OIDC_ISSUER_URL.")
	flag.StringVar(&clientID, "oidc-client-id", os.Getenv("OIDC_CLIENT_ID"), "The dashboard's OIDC client ID. Defaults to $OIDC_CLIENT_ID.")
	flag.StringVar(&caFile, "oidc-ca-file", "", "A PEM file of CA certificates to trust for the OIDC provider, besides the system's.")
	flag.StringVar(&scopes, "oidc-scopes", "openid,profile,email,groups,offline_access", "The scopes to request, comma-separated.")
	logOpts := zap.Options{}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	log := ctrl.Log.WithName("dashboard")
	ctx := ctrl.SetupSignalHandler()

	providerClient, err := providerHTTPClient(caFile)
	if err != nil {
		log.Error(err, "unable to read the OIDC provider's CA certificates")
		os.Exit(1)
	}

	handler, err := dashboard.New(ctx, dashboard.Options{
		PublicURL:    publicURL,
		IssuerURL:    issuerURL,
		ClientID:     clientID,
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		Scopes:       strings.Split(scopes, ","),
		Config:       ctrl.GetConfigOrDie(),
		HTTPClient:   providerClient,
	})
	if err != nil {
		log.Error(err, "unable to set up the dashboard")
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Requests end with the signal, so watch streams don't hold up shutting down; the browser reconnects to the next pod.
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
	shutDown := make(chan struct{})
	go func() {
		defer close(shutDown)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Error(err, "shutting down")
		}
	}()
	log.Info("serving", "address", addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "serving failed")
		os.Exit(1)
	}
	// Shutdown returns ListenAndServe at once, and then waits for requests to finish.
	<-shutDown
}

// providerHTTPClient returns the client for the OIDC provider, which also trusts the CA certificates in caFile. Without caFile, it's nil, for http.DefaultClient.
func providerHTTPClient(caFile string) (*http.Client, error) {
	if caFile == "" {
		return nil, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: no certificates found", caFile)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport}, nil
}
