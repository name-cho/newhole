package token

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/communication"
	authModels "github.com/name-cho/newhole/internal/pkg/token/models"
)

const tokensFileName = "tokens.json"
const maxTokenResponseBody = 1 << 20

var fileMutex sync.Mutex

type TokenStore struct {
	Token      authModels.TokenSpec `json:"token"`
	ObtainedAt time.Time            `json:"obtainedAt"`
	ExpiresAt  time.Time            `json:"expiresAt"`
	LastUsedAt time.Time            `json:"lastUsedAt"`
}

func tokenLocation() string {
	return cache.GetLocalStorageFile(tokensFileName, "")
}

func IsTokenSaved() bool {
	if config.Config.AuthToken != "" {
		return true
	}
	_, err := os.Stat(tokenLocation())
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		communication.Debug(fmt.Sprintf("Unable to read the token file: %s", err.Error()))
	}
	return false
}

func SaveToken(tokenSpec *authModels.TokenSpec) error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	if tokenSpec == nil {
		return errors.New("refusing to save an empty token")
	}

	previous, _ := readStoreUnlocked()
	lastUsed := time.Now()
	if previous != nil && !previous.LastUsedAt.IsZero() {
		lastUsed = previous.LastUsedAt
	}
	store := TokenStore{
		Token:      *tokenSpec,
		ObtainedAt: time.Now(),
		LastUsedAt: lastUsed,
	}
	if tokenSpec.ExpiresIn > 0 {
		store.ExpiresAt = store.ObtainedAt.Add(time.Duration(tokenSpec.ExpiresIn) * time.Second)
	}

	tokenBytes, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to encode the token: %w", err)
	}

	location := tokenLocation()
	temporary := location + ".tmp"
	if err := os.WriteFile(temporary, tokenBytes, 0o600); err != nil {
		return fmt.Errorf("unable to write the token file: %w", err)
	}
	if err := os.Rename(temporary, location); err != nil {
		os.Remove(temporary)
		return fmt.Errorf("unable to store the token file: %w", err)
	}
	return os.Chmod(location, 0o600)
}

func readStore() (*TokenStore, error) {
	fileMutex.Lock()
	defer fileMutex.Unlock()
	return readStoreUnlocked()
}

func readStoreUnlocked() (*TokenStore, error) {
	data, err := os.ReadFile(tokenLocation())
	if err != nil {
		return nil, err
	}

	store := TokenStore{}
	if err := json.Unmarshal(data, &store); err == nil && store.Token.AccessToken != "" {
		return &store, nil
	}

	legacy := authModels.TokenSpec{}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("the stored token file is corrupted (%w), run `newhole account logout` and log in again", err)
	}
	if legacy.AccessToken == "" {
		return nil, errors.New("the stored token file does not contain an access token")
	}
	return &TokenStore{Token: legacy, ObtainedAt: time.Now()}, nil
}

func readToken() (*authModels.TokenSpec, error) {
	store, err := readStore()
	if err != nil {
		return nil, err
	}
	return &store.Token, nil
}

func RegisterDevice() (*authModels.DeviceCodeSpec, error) {
	payload := url.Values{
		"client_id": {config.Config.OAuth.ClientID},
		"scope":     {config.Config.OAuth.Scope},
	}
	if config.Config.OAuth.Audience != "" {
		payload.Set("audience", config.Config.OAuth.Audience)
	}

	req, err := http.NewRequest(http.MethodPost, config.Config.OAuth.DeviceCodeURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("unable to create the device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := communication.HTTPClient(30 * time.Second)
	res, err := client.Do(req)
	if err != nil {
		hint := communication.ExplainNetworkError(err)
		if hint != "" {
			return nil, fmt.Errorf("unable to request a device code: %s (%s)", hint, err.Error())
		}
		return nil, fmt.Errorf("unable to request a device code: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxTokenResponseBody))
	if err != nil {
		return nil, fmt.Errorf("unable to read the device code response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("the authorisation server rejected the device code request (HTTP %d): %s; check that the system clock is correct, a skewed clock is the most common cause", res.StatusCode, summarize(body))
		}
		return nil, fmt.Errorf("the authorisation server rejected the device code request (HTTP %d): %s", res.StatusCode, summarize(body))
	}

	spec := authModels.DeviceCodeSpec{}
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("unable to decode the device code response (%s): %w", summarize(body), err)
	}
	if spec.DeviceCode == "" {
		return nil, fmt.Errorf("the authorisation server returned an empty device code: %s", summarize(body))
	}
	if spec.Interval <= 0 {
		spec.Interval = 5
	}
	if spec.VerificationURIComplete == "" && spec.VerificationURI != "" && spec.UserCode != "" {
		spec.VerificationURIComplete = spec.VerificationURI + "?user_code=" + url.QueryEscape(spec.UserCode)
	}
	return &spec, nil
}

func summarize(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}
	if len(trimmed) > 300 {
		return trimmed[:300] + "..."
	}
	return trimmed
}

func PollForToken(deviceCode string, interval int, quitChannel <-chan bool) (*authModels.TokenSpec, error) {
	grantType := "urn:ietf:params:oauth:grant-type:device_code"
	pollingInterval := time.Duration(interval) * time.Second
	if pollingInterval <= 0 {
		pollingInterval = 5 * time.Second
	}
	deadline := time.Now().Add(15 * time.Minute)
	client := communication.HTTPClient(30 * time.Second)

	for {
		select {
		case <-quitChannel:
			return nil, errors.New("login was aborted")
		default:
		}

		if time.Now().After(deadline) {
			return nil, errors.New("the login code expired before it was confirmed, run `newhole account login` again")
		}

		time.Sleep(pollingInterval)

		payload := url.Values{
			"grant_type":  {grantType},
			"device_code": {deviceCode},
			"client_id":   {config.Config.OAuth.ClientID},
		}
		req, err := http.NewRequest(http.MethodPost, config.Config.OAuth.TokenURL, strings.NewReader(payload.Encode()))
		if err != nil {
			return nil, fmt.Errorf("unable to build the token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		res, err := client.Do(req)
		if err != nil {
			communication.Debug(fmt.Sprintf("Token polling attempt failed: %s", err.Error()))
			continue
		}

		body, err := io.ReadAll(io.LimitReader(res.Body, maxTokenResponseBody))
		res.Body.Close()
		if err != nil {
			communication.Debug(fmt.Sprintf("Unable to read the token polling response: %s", err.Error()))
			continue
		}

		switch {
		case res.StatusCode >= 200 && res.StatusCode < 300:
			spec := authModels.TokenSpec{}
			if err := json.Unmarshal(body, &spec); err != nil {
				communication.Debug(fmt.Sprintf("Unable to decode the token response: %s", err.Error()))
				continue
			}
			if spec.AccessToken == "" {
				return nil, fmt.Errorf("the authorisation server did not return an access token: %s", summarize(body))
			}
			return &spec, nil

		case res.StatusCode >= 400 && res.StatusCode < 500:
			authErr := authModels.AuthError{}
			if err := json.Unmarshal(body, &authErr); err != nil {
				communication.Debug(fmt.Sprintf("Unable to decode the token error response: %s", err.Error()))
				continue
			}
			switch authErr.Error {
			case "authorization_pending":
				continue
			case "slow_down":
				pollingInterval += 5 * time.Second
				continue
			case "expired_token":
				return nil, errors.New("the login code expired, run `newhole account login` again")
			case "access_denied":
				return nil, errors.New("the login request was denied, run `newhole account login` again")
			case "invalid_grant":
				return nil, fmt.Errorf("the authorisation server rejected the login code: %s", authErr.ErrorDescription)
			default:
				return nil, fmt.Errorf("unexpected authorisation error %q: %s", authErr.Error, authErr.ErrorDescription)
			}

		default:
			return nil, fmt.Errorf("the authorisation server is unavailable (HTTP %d): %s", res.StatusCode, summarize(body))
		}
	}
}

func RefreshToken() error {
	if config.Config.AuthToken != "" {
		return nil
	}
	store, err := readStore()
	if err != nil {
		return fmt.Errorf("unable to refresh the session: %w", err)
	}
	if store.Token.RefreshToken == "" {
		return errors.New("no refresh token is stored, run `newhole account login` again")
	}

	payload := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {config.Config.OAuth.ClientID},
		"refresh_token": {store.Token.RefreshToken},
	}

	req, err := http.NewRequest(http.MethodPost, config.Config.OAuth.TokenURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := communication.HTTPClient(30 * time.Second)
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to refresh the session: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxTokenResponseBody))
	if err != nil {
		return fmt.Errorf("unable to read the refresh response: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		authErr := authModels.AuthError{}
		_ = json.Unmarshal(body, &authErr)
		if authErr.Error == "invalid_grant" || authErr.Error == "expired_token" || res.StatusCode == http.StatusUnauthorized {
			return errors.New("the session expired, run `newhole account login` again")
		}
		return fmt.Errorf("unable to refresh the session (HTTP %d): %s", res.StatusCode, summarize(body))
	}

	spec := authModels.TokenSpec{}
	if err := json.Unmarshal(body, &spec); err != nil {
		return fmt.Errorf("unable to decode the refresh response: %w", err)
	}
	if spec.RefreshToken == "" {
		spec.RefreshToken = store.Token.RefreshToken
	}
	if spec.IDToken == "" {
		spec.IDToken = store.Token.IDToken
	}
	return SaveToken(&spec)
}

func DeleteTokens() error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	location := tokenLocation()
	if err := os.Remove(location); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("unable to remove the token file: %w", err)
	}
	return nil
}

func Touch() {
	fileMutex.Lock()
	store, err := readStoreUnlocked()
	if err != nil || store == nil {
		fileMutex.Unlock()
		return
	}
	if !store.LastUsedAt.IsZero() && time.Since(store.LastUsedAt) < 5*time.Minute {
		fileMutex.Unlock()
		return
	}
	store.LastUsedAt = time.Now()
	data, err := json.MarshalIndent(store, "", "  ")
	if err == nil {
		_ = os.WriteFile(tokenLocation(), data, 0o600)
	}
	fileMutex.Unlock()
}

func LastUsedAt() time.Time {
	store, err := readStore()
	if err != nil || store == nil {
		return time.Time{}
	}
	return store.LastUsedAt
}

func GetAccessToken() (string, error) {
	if config.Config.AuthToken != "" {
		return config.Config.AuthToken, nil
	}
	tokenSpec, err := readToken()
	if err != nil {
		return "", err
	}
	if tokenSpec.AccessToken == "" {
		return "", errors.New("the stored token does not contain an access token")
	}
	return tokenSpec.AccessToken, nil
}

func GetRefreshToken() (string, error) {
	tokenSpec, err := readToken()
	if err != nil {
		return "", err
	}
	return tokenSpec.RefreshToken, nil
}

func GetIdToken() string {
	tokenSpec, err := readToken()
	if err != nil {
		return ""
	}
	return tokenSpec.IDToken
}

func Claims() map[string]interface{} {
	idToken := GetIdToken()
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	claims := map[string]interface{}{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims
}

func ExpiresAt() (time.Time, bool) {
	store, err := readStore()
	if err != nil || store.ExpiresAt.IsZero() {
		return time.Time{}, false
	}
	return store.ExpiresAt, true
}
