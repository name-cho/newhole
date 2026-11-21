package models

import "testing"

func TestEndpointURIAndHostname(t *testing.T) {
	cases := []struct {
		endpoint Endpoint
		uri      string
		hostname string
	}{
		{Endpoint{Host: "127.0.0.1", Port: 3000}, "127.0.0.1:3000", "127.0.0.1:3000"},
		{Endpoint{Protocol: "http", Host: "127.0.0.1", Port: 3000, Path: "/app"}, "http://127.0.0.1:3000/app", "127.0.0.1:3000"},
		{Endpoint{Protocol: "https", Host: "example.com", Port: 443}, "https://example.com", "example.com"},
		{Endpoint{Protocol: "ssh", Host: "::1", Port: 8022}, "ssh://[::1]:8022", "[::1]:8022"},
		{Endpoint{Protocol: "ssh", Host: "[::1]", Port: 8022}, "ssh://[::1]:8022", "[::1]:8022"},
	}
	for _, testCase := range cases {
		if got := testCase.endpoint.URI(); got != testCase.uri {
			t.Fatalf("URI %q differs from %q", got, testCase.uri)
		}
		if got := testCase.endpoint.Hostname(); got != testCase.hostname {
			t.Fatalf("Hostname %q differs from %q", got, testCase.hostname)
		}
	}
}

func TestEndpointURL(t *testing.T) {
	endpoint := Endpoint{Protocol: "https"}
	if got := endpoint.URL("site", "loophole.site"); got != "https://site.loophole.site" {
		t.Fatalf("URL %q differs from expected", got)
	}
}

func TestNormalizeHostnameLowercases(t *testing.T) {
	specs := RemoteEndpointSpecs{SiteID: "  MixedCase "}
	specs.NormalizeHostname()
	if specs.SiteID != "mixedcase" {
		t.Fatalf("hostname %q was not normalized", specs.SiteID)
	}
}

func TestValidateLocalHTTP(t *testing.T) {
	if err := Validate(&LocalHTTPEndpointSpecs{Port: 0, Host: ""}); err == nil {
		t.Fatal("expected an error for a missing port")
	}
	if err := Validate(&LocalHTTPEndpointSpecs{Port: 3000}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Validate(&LocalHTTPEndpointSpecs{UnixSocket: "/tmp/app.sock"}); err != nil {
		t.Fatalf("unexpected error for unix socket: %v", err)
	}
}
