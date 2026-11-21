package urlmaker

import "testing"

func TestGetSiteURLCases(t *testing.T) {
	cases := []struct {
		protocol string
		siteID   string
		domain   string
		expected string
	}{
		{"https", "some-site", "loophole.site", "https://some-site.loophole.site"},
		{"https", "some-site", "loophole.host", "https://some-site.loophole.host"},
		{"webdav", "some-site", "loophole.site", "webdav://some-site.loophole.site"},
		{"davs", "some-site", "loophole.host", "davs://some-site.loophole.host"},
		{"https", "MixedCase", "loophole.site", "https://mixedcase.loophole.site"},
	}
	for _, testCase := range cases {
		if got := GetSiteURL(testCase.protocol, testCase.siteID, testCase.domain); got != testCase.expected {
			t.Fatalf("URL %q differs from expected %q", got, testCase.expected)
		}
	}
}

func TestGetSiteFQDNCases(t *testing.T) {
	if got := GetSiteFQDN("some-site", "loophole.site"); got != "some-site.loophole.site" {
		t.Fatalf("FQDN %q differs from expected", got)
	}
	if got := GetSiteFQDN("some-site", ""); got != "some-site" {
		t.Fatalf("FQDN %q differs from expected", got)
	}
}
