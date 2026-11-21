package newhole

import (
	"strings"
	"testing"
	"time"
)

func TestExplainSSHErrorIsActionable(t *testing.T) {
	cases := map[string]string{
		"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey]": "newhole account login",
		"dial tcp 1.2.3.4:8022: i/o timeout":                                                     "unreachable",
		"lookup gateway.loophole.host: no such host":                                             "resolved",
		"ssh: tcpip-forward request denied by peer":                                              "denied",
		"read tcp: connection reset by peer":                                                     "dropped",
	}
	for message, expected := range cases {
		explanation := explainSSHError(errorString(message))
		if !strings.Contains(explanation, expected) {
			t.Fatalf("explanation %q does not contain %q", explanation, expected)
		}
	}
}

func TestDurationFromConfigFallsBack(t *testing.T) {
	if got := durationFromConfig("", time.Second); got != time.Second {
		t.Fatalf("duration %v differs from expected", got)
	}
	if got := durationFromConfig("garbage", 2*time.Second); got != 2*time.Second {
		t.Fatalf("duration %v differs from expected", got)
	}
	if got := durationFromConfig("5s", time.Second); got != 5*time.Second {
		t.Fatalf("duration %v differs from expected", got)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
