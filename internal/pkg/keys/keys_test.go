package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestParsePublicKeyGeneratesEd25519Key(t *testing.T) {
	directory := t.TempDir()
	identity := filepath.Join(directory, ".ssh", "id_rsa")

	authMethod, publicKey, err := ParsePublicKey(identity)
	if err != nil {
		t.Fatal(err)
	}
	if authMethod == nil || publicKey == nil {
		t.Fatal("expected an auth method and a public key")
	}
	if publicKey.Type() != "ssh-ed25519" {
		t.Fatalf("generated key type %q is not ed25519", publicKey.Type())
	}

	info, err := os.Stat(identity)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key permissions %v differ from 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(identity + ".pub"); err != nil {
		t.Fatal("public key was not written")
	}

	if _, _, err := ParsePublicKey(identity); err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
}

func TestParsePublicKeyConvertsLegacyRSAKey(t *testing.T) {
	directory := t.TempDir()
	identity := filepath.Join(directory, "id_legacy")

	pemBlock := []byte(testRSAKey)
	if err := os.WriteFile(identity, pemBlock, 0o600); err != nil {
		t.Fatal(err)
	}

	_, publicKey, err := ParsePublicKey(identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("key parsed")
	if publicKey.Type() != "ssh-rsa" {
		t.Fatalf("converted key type %q is not ssh-rsa", publicKey.Type())
	}
}

func TestConvertLegacyRSAKey(t *testing.T) {
	converted, err := convertLegacyRSAPrivateKey([]byte(testRSAKey))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(converted), "BEGIN RSA PRIVATE KEY") {
		t.Fatal("the legacy key was not converted")
	}
	if _, err := ssh.ParsePrivateKey(converted); err != nil {
		t.Fatalf("converted key cannot be parsed: %v", err)
	}
}

func TestFingerprintIsStable(t *testing.T) {
	directory := t.TempDir()
	_, publicKey, err := ParsePublicKey(filepath.Join(directory, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(publicKey) != Fingerprint(publicKey) {
		t.Fatal("fingerprint is not stable")
	}
}

const testRSAKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf9Cnzj4p4WGeKLs1Pt8Qu
KUpRKfFLfRYC9AIKjbJTWit+CqvjWYzvQwECAwEAAQJAIJLixBy2qpFoS4DSmoEm
o3qGy0t6z09AIJtH+5OeRV1be+N4cDYJKffGzDa88vQENZiRm0GRq6a+HPGQMd2k
TQIhAKMSvzIBnni7ot/OSie2TmJLY4SwTQAevXysE2RbFDYdAiEBCUEaRQnMnbp7
9mxDXDf6AU0cN/RPBjb9qSHDcWZHGzUCIG2Es59z8ugGrDY+pxLQnwfotadxd+Uy
v/Ow5T0q5gIJAiEAyS4RaI9YG8EWx/2w0T67ZUVAw8eOMB6BIUg0Xcu+3okCIBOs
/5OiPgoTdSy7bcF9IGpSE8ZgGKzgYQVZeN97YE00
-----END RSA PRIVATE KEY-----
`
