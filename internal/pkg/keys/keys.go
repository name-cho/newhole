package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/name-cho/newhole/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

type KeySource int

const (
	SourceFile KeySource = iota
	SourceAgent
	SourceGenerated
)

var promptMutex sync.Mutex

func ParsePublicKey(file string) (ssh.AuthMethod, ssh.PublicKey, error) {
	privateKey, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		privateKey, err = generateEd25519Key()
		if err != nil {
			return nil, nil, err
		}
		if err := writeKeyPair(file, privateKey); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, fmt.Errorf("unable to read the identity file %s: %w", file, err)
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err == nil {
		return ssh.PublicKeys(signer), signer.PublicKey(), nil
	}
	if strings.Contains(err.Error(), "no key found") {
		converted, conversionErr := convertLegacyRSAPrivateKey(privateKey)
		if conversionErr == nil {
			if convertedSigner, parseErr := ssh.ParsePrivateKey(converted); parseErr == nil {
				_ = os.WriteFile(file, converted, 0o600)
				return ssh.PublicKeys(convertedSigner), convertedSigner.PublicKey(), nil
			}
		}
	}

	var passphraseMissing *ssh.PassphraseMissingError
	if !errors.As(err, &passphraseMissing) {
		return nil, nil, fmt.Errorf("unable to parse the identity file %s: %w", file, err)
	}

	if agentSigner, agentErr := signerFromAgent(passphraseMissing.PublicKey); agentErr == nil {
		return ssh.PublicKeys(agentSigner), agentSigner.PublicKey(), nil
	}

	password, passwordErr := readPassphrase()
	if passwordErr != nil {
		return nil, nil, passwordErr
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, password)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to unlock the identity file %s: %w", file, err)
	}
	return ssh.PublicKeys(signer), signer.PublicKey(), nil
}

func readPassphrase() ([]byte, error) {
	promptMutex.Lock()
	defer promptMutex.Unlock()

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("the SSH key is passphrase protected and no terminal is available to ask for it; use an unencrypted key or add it to ssh-agent")
	}
	fmt.Print("Enter SSH key passphrase: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, err
	}
	return password, nil
}

func signerFromAgent(expected ssh.PublicKey) (ssh.Signer, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, errors.New("SSH_AUTH_SOCK is not set")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	client := agent.NewClient(conn)
	signers, err := client.Signers()
	if err != nil {
		return nil, err
	}
	for _, signer := range signers {
		if expected == nil || string(signer.PublicKey().Marshal()) == string(expected.Marshal()) {
			return signer, nil
		}
	}
	return nil, errors.New("the key was not found in ssh-agent")
}

func generateEd25519Key() ([]byte, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func writeKeyPair(file string, privateKey []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(file, privateKey, 0o600); err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return err
	}
	publicKey := ssh.MarshalAuthorizedKey(signer.PublicKey())
	return os.WriteFile(file+".pub", publicKey, 0o644)
}

func Fingerprint(publicKey ssh.PublicKey) string {
	hash := fnv.New64a()
	hash.Write(publicKey.Marshal())
	return fmt.Sprintf("NEWHOLE:%08x", hash.Sum64())
}

func HostKeyCallback() (ssh.HostKeyCallback, error) {
	policy := strings.ToLower(strings.TrimSpace(config.Config.HostKeyPolicy))
	if policy == "" || policy == "insecure" || policy == "ignore" {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsFile := config.Config.KnownHosts
	if knownHostsFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("unable to locate the home directory: %w", err)
		}
		knownHostsFile = filepath.Join(home, ".ssh", "known_hosts")
	}
	if _, err := os.Stat(knownHostsFile); err != nil {
		return nil, fmt.Errorf("host key verification is %q but %s is not readable: %w", policy, knownHostsFile, err)
	}

	return knownhosts.New(knownHostsFile)
}

func convertLegacyRSAPrivateKey(data []byte) ([]byte, error) {
	blocks := [][]byte{data}
	for len(blocks) > 0 {
		block, rest := pem.Decode(blocks[0])
		blocks = blocks[1:]
		if rest != nil {
			blocks = append(blocks, rest)
		}
		if block == nil {
			continue
		}
		switch block.Type {
		case "RSA PRIVATE KEY":
			key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				continue
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				return nil, err
			}
			return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
		case "EC PRIVATE KEY":
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				continue
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				return nil, err
			}
			return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
		}
	}
	return nil, errors.New("no legacy private key found")
}
