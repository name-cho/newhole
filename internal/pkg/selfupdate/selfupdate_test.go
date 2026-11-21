package selfupdate

import (
	"runtime"
	"testing"
)

func platformSuffix(desktop bool) string {
	extension := "tar.gz"
	if runtime.GOOS == "windows" {
		extension = "zip"
	}
	return runtime.GOOS + "_" + runtime.GOARCH + "." + extension
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current   string
		candidate string
		expected  bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.1", "1.0.0", false},
		{"development", "1.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"v1.2.3", "1.3.0", true},
		{"1.0.0-beta.1", "1.0.0-beta.2", false},
		{"0.0.0", "1.0.0", true},
	}
	for _, testCase := range cases {
		if got := isNewer(testCase.current, testCase.candidate); got != testCase.expected {
			t.Fatalf("isNewer(%q, %q) = %v, expected %v", testCase.current, testCase.candidate, got, testCase.expected)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := assetName("v1.2.3", false); got == "" || got != "newhole_1.2.3_"+platformSuffix(false) {
		t.Fatalf("unexpected asset name %q", got)
	}
	if got := assetName("v1.2.3", true); got != "newhole-desktop_1.2.3_"+platformSuffix(true) {
		t.Fatalf("unexpected asset name %q", got)
	}
}
