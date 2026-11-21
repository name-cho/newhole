package httpserver

import "os"

func writeFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
