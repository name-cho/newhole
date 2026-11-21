package token

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/cache"
	authModels "github.com/name-cho/newhole/internal/pkg/token/models"
)

func setupHome(t *testing.T) {
	t.Helper()
	t.Setenv("NEWHOLE_HOME", t.TempDir())
}

func TestSaveTokenWritesReadableFile(t *testing.T) {
	setupHome(t)
	if err := SaveToken(&authModels.TokenSpec{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	location := cache.GetLocalStorageFile(tokensFileName, "")
	info, err := os.Stat(location)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token file permissions %v differ from 0600", info.Mode().Perm())
	}
	accessToken, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if accessToken != "access" {
		t.Fatalf("access token %q differs from expected", accessToken)
	}
}

func TestLegacyTokenFileIsReadable(t *testing.T) {
	setupHome(t)
	location := cache.GetLocalStorageFile(tokensFileName, "")
	if err := os.WriteFile(location, []byte(`{"access_token":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	accessToken, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if accessToken != "legacy" {
		t.Fatalf("legacy access token %q was not read", accessToken)
	}
}

func TestCorruptedTokenFileGivesActionableError(t *testing.T) {
	setupHome(t)
	location := cache.GetLocalStorageFile(tokensFileName, "")
	if err := os.WriteFile(location, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := GetAccessToken()
	if err == nil {
		t.Fatal("expected an error")
	}
	message := err.Error()
	if !contains(message, "newhole account logout") {
		t.Fatalf("error %q does not tell the user what to do", message)
	}
}

func TestConfigAuthTokenOverridesStoredFile(t *testing.T) {
	setupHome(t)
	t.Setenv("NEWHOLE_HOME", filepath.Join(t.TempDir(), "home"))
	old := config.Config.AuthToken
	config.Config.AuthToken = "from-env"
	defer func() { config.Config.AuthToken = old }()

	if !IsTokenSaved() {
		t.Fatal("a configured token should count as logged in")
	}
	accessToken, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if accessToken != "from-env" {
		t.Fatalf("access token %q differs from expected", accessToken)
	}
}

func TestPollForTokenHandlesPendingThenSuccess(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(authModels.AuthError{Error: "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(authModels.TokenSpec{AccessToken: "new-token"})
	}))
	defer server.Close()

	old := config.Config.OAuth.TokenURL
	config.Config.OAuth.TokenURL = server.URL
	defer func() { config.Config.OAuth.TokenURL = old }()

	tokens, err := PollForToken("device", 0, make(chan bool))
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "new-token" {
		t.Fatalf("token %q differs from expected", tokens.AccessToken)
	}
}

func TestRegisterDeviceReportsServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found."))
	}))
	defer server.Close()

	old := config.Config.OAuth.DeviceCodeURL
	config.Config.OAuth.DeviceCodeURL = server.URL
	defer func() { config.Config.OAuth.DeviceCodeURL = old }()

	_, err := RegisterDevice()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !contains(err.Error(), "404") {
		t.Fatalf("error %q does not include the status code", err.Error())
	}
}

func contains(value string, needle string) bool {
	return len(value) >= len(needle) && (value == needle || len(needle) == 0 || indexOf(value, needle) >= 0)
}

func indexOf(value string, needle string) int {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
