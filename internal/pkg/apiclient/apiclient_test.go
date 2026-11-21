package apiclient

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/ssh"
)

func getPublicSSHKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey
}

func serverMock(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

func withMocks(t *testing.T) {
	t.Helper()
	oldIsTokenSaved := isTokenSaved
	oldGetAccessToken := getAccessToken
	oldURL := apiURL
	oldRefreshed := tokenWasRefreshed
	t.Cleanup(func() {
		isTokenSaved = oldIsTokenSaved
		getAccessToken = oldGetAccessToken
		apiURL = oldURL
		tokenWasRefreshed = oldRefreshed
	})
	isTokenSaved = func() bool { return true }
	getAccessToken = func() (string, error) { return "some-token", nil }
	tokenWasRefreshed = false
}

func TestRegisterSiteSuccessOKShouldPropagateWithoutIDProvided(t *testing.T) {
	withMocks(t)
	srv := serverMock(http.StatusOK, `{"siteId":"randomidwhichissuperlong","domain":"loophole.site"}`)
	defer srv.Close()
	SetAPIURL(srv.URL)

	result, err := RegisterSite(getPublicSSHKey(t), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SiteID != "randomidwhichissuperlong" {
		t.Fatalf("site id %q differs from expected", result.SiteID)
	}
}

func TestRegisterSiteSuccessCreatedShouldPropagateWithIDProvided(t *testing.T) {
	withMocks(t)
	srv := serverMock(http.StatusCreated, `{"siteId":"providedhostname","domain":"loophole.site"}`)
	defer srv.Close()
	SetAPIURL(srv.URL)

	result, err := RegisterSite(getPublicSSHKey(t), "providedhostname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SiteID != "providedhostname" {
		t.Fatalf("site id %q differs from expected", result.SiteID)
	}
}

func TestRegisterSiteLowercasesRequestedHostname(t *testing.T) {
	withMocks(t)
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = decodeJSON(r, &payload)
		received = payload["id"]
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"siteId":"mixedcase","domain":"loophole.site"}`)
	}))
	defer srv.Close()
	SetAPIURL(srv.URL)

	if _, err := RegisterSite(getPublicSSHKey(t), "MixedCase"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "mixedcase" {
		t.Fatalf("hostname %q was not lowercased", received)
	}
}

func TestRegisterSiteConflictExplainsTheReason(t *testing.T) {
	withMocks(t)
	srv := serverMock(http.StatusConflict, `{"message":"Site exists","error":"conflict"}`)
	defer srv.Close()
	SetAPIURL(srv.URL)

	_, err := RegisterSite(getPublicSSHKey(t), "taken")
	if err == nil {
		t.Fatal("expected an error")
	}
	requestErr, ok := err.(RequestError)
	if !ok {
		t.Fatalf("expected RequestError, got %T", err)
	}
	if requestErr.StatusCode != http.StatusConflict {
		t.Fatalf("status %d differs from expected", requestErr.StatusCode)
	}
	if requestErr.Hint == "" {
		t.Fatal("expected a hint explaining how to fix the conflict")
	}
}

func TestRegisterSiteHTMLResponseIsExplained(t *testing.T) {
	withMocks(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "<html><body>cloudflare error</body></html>")
	}))
	defer srv.Close()
	SetAPIURL(srv.URL)

	_, err := RegisterSite(getPublicSSHKey(t), "")
	if err == nil {
		t.Fatal("expected an error")
	}
	requestErr, ok := err.(RequestError)
	if !ok {
		t.Fatalf("expected RequestError, got %T: %v", err, err)
	}
	if requestErr.Message == "" {
		t.Fatal("expected a human readable message for the HTML response")
	}
}

func TestRegisterSiteWithoutTokenAsksToLogin(t *testing.T) {
	withMocks(t)
	isTokenSaved = func() bool { return false }

	_, err := RegisterSite(getPublicSSHKey(t), "")
	if err == nil {
		t.Fatal("expected an error")
	}
	requestErr, ok := err.(RequestError)
	if !ok {
		t.Fatalf("expected RequestError, got %T", err)
	}
	if requestErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d differs from expected", requestErr.StatusCode)
	}
}
