package apiclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"golang.org/x/crypto/ssh"
)

type RegistrationSuccessResponse struct {
	SiteID string `json:"siteId"`
	Domain string `json:"domain"`
}

type InfoSuccessResponse struct {
	Version string `json:"version"`
}

type ErrorResponse struct {
	StatusCode int32  `json:"statusCode"`
	Message    string `json:"message"`
	Error      string `json:"error"`
	Details    string `json:"details"`
}

type RequestError struct {
	Message    string
	Details    string
	Hint       string
	StatusCode int
}

func (err RequestError) Error() string {
	parts := []string{fmt.Sprintf("Request Error (%d)", err.StatusCode)}
	if err.Message != "" {
		parts = append(parts, err.Message)
	}
	if err.Details != "" {
		parts = append(parts, err.Details)
	}
	return strings.Join(parts, ": ")
}

const (
	maxResponseBody = 1 << 20
	maxAttempts     = 3
)

var (
	isTokenSaved   = token.IsTokenSaved
	getAccessToken = token.GetAccessToken

	refreshMutex      sync.Mutex
	tokenWasRefreshed bool

	apiURL string
	apiMu  sync.Mutex
)

func baseURL() string {
	apiMu.Lock()
	defer apiMu.Unlock()
	if apiURL == "" {
		apiURL = config.Config.APIEndpoint.URI()
	}
	return apiURL
}

func SetAPIURL(value string) {
	apiMu.Lock()
	defer apiMu.Unlock()
	apiURL = value
}

func userAgent() string {
	return fmt.Sprintf("newhole-%s/%s-%s (%s/%s)",
		config.Config.ClientMode,
		config.Config.Version,
		config.Config.CommitHash,
		runtime.GOOS,
		runtime.GOARCH,
	)
}

func hintForTransportError(err error) string {
	if err == nil {
		return ""
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "The request timed out. Check your internet connection or whether the Newhole API is reachable"
	}
	switch {
	case strings.Contains(err.Error(), "no such host"):
		return "DNS lookup failed. Check your network and DNS configuration"
	case strings.Contains(err.Error(), "certificate"):
		return "TLS verification failed. If you use a proxy that intercepts TLS, point NEWHOLE_CA_BUNDLE at its root CA"
	case strings.Contains(err.Error(), "connection refused"):
		return "Connection refused by the API host"
	case strings.Contains(err.Error(), "connection reset"):
		return "Connection was reset while talking to the API"
	case strings.Contains(err.Error(), "invalid character"), strings.Contains(err.Error(), "unexpected end of json"):
		return "The API answered with something that is not JSON, most likely an HTML page from a captive portal or a proxy. Open https://api.loophole.cloud/api/info in a browser to confirm"
	}
	return communication.ExplainNetworkError(err)
}

func doRequest(req *http.Request) (*http.Response, error) {
	client := communication.HTTPClient(45 * time.Second)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		clone := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err == nil {
				clone.Body = body
			}
		}
		resp, err := client.Do(clone)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var netErr net.Error
		retryable := errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
		if attempt < maxAttempts && retryable {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		break
	}
	return nil, lastErr
}

func decodeErrorBody(resp *http.Response) ErrorResponse {
	errorResponse := ErrorResponse{}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return errorResponse
	}
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		contentType := resp.Header.Get("Content-Type")
		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") || strings.Contains(contentType, "html") {
			errorResponse.Message = "The API host answered with an HTML page instead of JSON. The service is probably down, behind a captive portal or replaced by a proxy"
		} else if trimmed == "" {
			errorResponse.Message = fmt.Sprintf("The API host answered with an empty response (HTTP %d)", resp.StatusCode)
		} else {
			if len(trimmed) > 200 {
				trimmed = trimmed[:200] + "..."
			}
			errorResponse.Message = fmt.Sprintf("Unexpected response from the API host (HTTP %d): %s", resp.StatusCode, trimmed)
		}
	}
	return errorResponse
}

func RegisterSite(publicKey ssh.PublicKey, requestedSiteID string) (*RegistrationSuccessResponse, error) {
	publicKeyString := publicKey.Type() + " " + base64.StdEncoding.EncodeToString(publicKey.Marshal())

	if config.Config.AuthToken == "" && !isTokenSaved() {
		return nil, RequestError{
			Message:    "You are not logged in",
			Details:    "No locally stored credentials were found",
			Hint:       "Run `newhole account login` or set NEWHOLE_TOKEN",
			StatusCode: http.StatusUnauthorized,
		}
	}

	accessToken := config.Config.AuthToken
	if accessToken == "" {
		token.Touch()
		var err error
		accessToken, err = getAccessToken()
		if err != nil || accessToken == "" {
			return nil, RequestError{
				Message:    "There was a problem reading the stored token",
				Details:    errorText(err),
				Hint:       "Run `newhole account login` again",
				StatusCode: http.StatusUnauthorized,
			}
		}
	}

	data := map[string]string{
		"key": publicKeyString,
	}
	if requestedSiteID != "" {
		data["id"] = strings.ToLower(strings.TrimSpace(requestedSiteID))
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s/api/site", baseURL())
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonData)), nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := doRequest(req)
	if err != nil {
		return nil, RequestError{
			Message:    fmt.Sprintf("Unable to reach the Newhole API at %s", baseURL()),
			Details:    err.Error(),
			Hint:       hintForTransportError(err),
			StatusCode: 0,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errorResponse := decodeErrorBody(resp)

		switch resp.StatusCode {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return nil, RequestError{
				Message:    firstNonEmpty(errorResponse.Message, "The requested hostname is not valid"),
				Details:    hostnameRequirements,
				Hint:       "Hostname must be lowercase, so Newhole lowercases it for you",
				StatusCode: resp.StatusCode,
			}
		case http.StatusUnauthorized:
			if config.Config.AuthToken == "" {
				if refreshErr := refreshOnce(); refreshErr == nil {
					return RegisterSite(publicKey, requestedSiteID)
				} else {
					communication.Debug("token refresh failed: " + refreshErr.Error())
				}
			}
			return nil, RequestError{
				Message:    "Authentication failed",
				Details:    firstNonEmpty(errorResponse.Message, "The stored credentials were rejected"),
				Hint:       "Run `newhole account logout` and `newhole account login`",
				StatusCode: resp.StatusCode,
			}
		case http.StatusForbidden:
			return nil, RequestError{
				Message:    "You do not have permission to establish a tunnel with these parameters",
				Details:    firstNonEmpty(errorResponse.Message, errorResponse.Error),
				Hint:       "Check your plan limits and the requested hostname",
				StatusCode: resp.StatusCode,
			}
		case http.StatusConflict:
			return nil, RequestError{
				Message:    "The requested hostname is already used by another account",
				Details:    firstNonEmpty(errorResponse.Message, errorResponse.Error),
				Hint:       "Pick a different --hostname, or unregister the old one",
				StatusCode: resp.StatusCode,
			}
		case http.StatusTooManyRequests:
			return nil, RequestError{
				Message:    "Too many requests to the Newhole API",
				Details:    firstNonEmpty(errorResponse.Message, errorResponse.Error),
				Hint:       "Wait a moment before retrying",
				StatusCode: resp.StatusCode,
			}
		case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			return nil, RequestError{
				Message:    "The Newhole API is temporarily unavailable",
				Details:    firstNonEmpty(errorResponse.Message, errorResponse.Error),
				Hint:       "Retry in a few seconds",
				StatusCode: resp.StatusCode,
			}
		default:
			if resp.StatusCode >= 500 {
				return nil, RequestError{
					Message:    "The Newhole API failed on its side",
					Details:    firstNonEmpty(errorResponse.Details, errorResponse.Message, errorResponse.Error, "the server did not say what broke"),
					Hint:       "Newhole already retried; wait a few seconds and start the tunnel again",
					StatusCode: resp.StatusCode,
				}
			}
			return nil, RequestError{
				Message:    firstNonEmpty(errorResponse.Message, "Unexpected error from the Newhole API"),
				Details:    firstNonEmpty(errorResponse.Details, errorResponse.Error, "the server did not explain what went wrong"),
				Hint:       "Retry, and if the problem persists report it with `newhole feedback`",
				StatusCode: resp.StatusCode,
			}
		}
	}

	result := RegistrationSuccessResponse{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&result); err != nil {
		return nil, fmt.Errorf("unable to decode the site registration response: %w", err)
	}
	if result.SiteID == "" {
		return nil, fmt.Errorf("the site registration response did not contain a site id")
	}
	result.SiteID = strings.ToLower(result.SiteID)

	communication.Debug(fmt.Sprintf("Site registration response: %v", result))
	return &result, nil
}

const hostnameRequirements = `The hostname must:
- start with a letter
- contain only lowercase letters, digits and single dashes between them
- end with a letter or a digit`

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func refreshOnce() error {
	refreshMutex.Lock()
	defer refreshMutex.Unlock()
	if tokenWasRefreshed {
		return fmt.Errorf("token was already refreshed once")
	}
	if err := token.RefreshToken(); err != nil {
		return err
	}
	tokenWasRefreshed = true
	return nil
}

func GetLatestAvailableVersion() (*InfoSuccessResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/info", baseURL()), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent())

	resp, err := doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request for version information failed with status %d", resp.StatusCode)
	}

	result := InfoSuccessResponse{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&result); err != nil {
		return nil, err
	}
	communication.Debug(fmt.Sprintf("Info response: %v", result))
	return &result, nil
}

func ResolveHostname(siteID string, domain string) string {
	if siteID == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", strings.ToLower(siteID), domain)
}

func ParseEndpoint(value string) (*url.URL, error) {
	return url.Parse(value)
}
