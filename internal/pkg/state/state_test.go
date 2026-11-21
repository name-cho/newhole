package state

import (
	"os"
	"testing"

	"github.com/name-cho/newhole/internal/pkg/cache"
)

func TestHostnameHistoryRoundTrip(t *testing.T) {
	os.Setenv("NEWHOLE_HOME", t.TempDir())
	defer os.Unsetenv("NEWHOLE_HOME")

	if names := HostnameNames(); len(names) != 0 {
		t.Fatalf("expected an empty history, got %v", names)
	}

	if err := RecordHostname("first", "loophole.site", "http"); err != nil {
		t.Fatal(err)
	}
	if err := RecordHostname("second", "loophole.site", "path"); err != nil {
		t.Fatal(err)
	}
	if err := RecordHostname("first", "loophole.site", "http"); err != nil {
		t.Fatal(err)
	}

	records := Hostnames()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Hostname != "first" {
		t.Fatalf("expected the most recently used hostname first, got %q", records[0].Hostname)
	}
	if records[0].Uses != 2 {
		t.Fatalf("expected 2 uses, got %d", records[0].Uses)
	}

	location := cache.GetLocalStorageFile("hostnames.json", "")
	info, err := os.Stat(location)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("history permissions %v differ from 0600", info.Mode().Perm())
	}

	if err := ForgetHostname("second"); err != nil {
		t.Fatal(err)
	}
	if names := HostnameNames(); len(names) != 1 || names[0] != "first" {
		t.Fatalf("expected only the first hostname, got %v", names)
	}
}

func TestLastTunnelSettings(t *testing.T) {
	os.Setenv("NEWHOLE_HOME", t.TempDir())
	defer os.Unsetenv("NEWHOLE_HOME")

	if settings := LastTunnelSettings(); settings != nil {
		t.Fatalf("expected no settings, got %+v", settings)
	}

	if err := SaveLastTunnel(LastTunnel{Mode: "http", Target: "3000", Hostname: "demo"}); err != nil {
		t.Fatal(err)
	}
	settings := LastTunnelSettings()
	if settings == nil {
		t.Fatal("expected the stored settings")
	}
	if settings.Mode != "http" || settings.Target != "3000" || settings.Hostname != "demo" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestCorruptedHistoryIsIgnored(t *testing.T) {
	os.Setenv("NEWHOLE_HOME", t.TempDir())
	defer os.Unsetenv("NEWHOLE_HOME")

	location := cache.GetLocalStorageFile("hostnames.json", "")
	if err := os.WriteFile(location, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if names := HostnameNames(); len(names) != 0 {
		t.Fatalf("expected an empty history, got %v", names)
	}
	if err := RecordHostname("recovered", "loophole.site", "http"); err != nil {
		t.Fatal(err)
	}
	if names := HostnameNames(); len(names) != 1 {
		t.Fatalf("expected the history to be rewritten, got %v", names)
	}
}
