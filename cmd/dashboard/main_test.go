package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderHTTPClientTrustsCAFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := http.Get(server.URL); err == nil {
		t.Fatal("the default client trusts the test server, so this test can't tell")
	}
	client, err := providerHTTPClient(caFile)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("getting from a server signed by the CA file: %v", err)
	}
	_ = resp.Body.Close()
}

func TestProviderHTTPClientRejectsFileWithoutCertificates(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := providerHTTPClient(caFile); err == nil {
		t.Error("got no error for a file without certificates")
	}
}
