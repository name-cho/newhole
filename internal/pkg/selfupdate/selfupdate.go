package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/communication"
)

const (
	latestReleaseURL  = "https://api.github.com/repos/name-cho/newhole/releases/latest"
	maxDownloadSize   = 200 << 20
	checksumsFileName = "checksums.txt"
)

type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

type release struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Body    string         `json:"body"`
	Assets  []releaseAsset `json:"assets"`
}

type Result struct {
	Version string
	Path    string
	Updated bool
	Skipped string
}

func client() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func fetchRelease() (*release, error) {
	request, err := http.NewRequest(http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "newhole/"+config.Config.Version)

	response, err := client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("unable to reach the release feed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil, errors.New("no published release is available yet")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the release feed answered with status %d", response.StatusCode)
	}

	parsed := release{}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("unable to decode the release feed: %w", err)
	}
	return &parsed, nil
}

func assetName(version string, desktop bool) string {
	platform := runtime.GOOS
	architecture := runtime.GOARCH
	name := "newhole"
	if desktop {
		name = "newhole-desktop"
	}
	extension := "tar.gz"
	if platform == "windows" {
		extension = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", name, strings.TrimPrefix(version, "v"), platform, architecture, extension)
}

func selectAsset(releases *release, desktop bool) (*releaseAsset, error) {
	expected := assetName(releases.TagName, desktop)
	for index := range releases.Assets {
		asset := releases.Assets[index]
		if asset.Name == expected {
			return &asset, nil
		}
	}
	alternatives := make([]string, 0, len(releases.Assets))
	for _, asset := range releases.Assets {
		alternatives = append(alternatives, asset.Name)
	}
	if len(alternatives) == 0 {
		return nil, fmt.Errorf("the release %s does not contain any assets", releases.TagName)
	}
	return nil, fmt.Errorf("the release %s does not contain %s (available: %s)", releases.TagName, expected, strings.Join(alternatives, ", "))
}

func download(asset *releaseAsset) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "newhole/"+config.Config.Version)

	response, err := client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("unable to download %s: %w", asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the download of %s answered with status %d", asset.Name, response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxDownloadSize))
}

func extract(payload []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if file.FileInfo().IsDir() {
				continue
			}
			if file.FileInfo().IsDir() || !strings.HasSuffix(file.Name, binaryBaseName(name)) {
				continue
			}
			handle, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer handle.Close()
			return io.ReadAll(io.LimitReader(handle, maxDownloadSize))
		}
		return nil, fmt.Errorf("%s does not contain the binary", name)
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if !strings.Contains(header.Name, binaryBaseName(name)) {
			continue
		}
		return io.ReadAll(io.LimitReader(tarReader, maxDownloadSize))
	}
	return nil, fmt.Errorf("%s does not contain the binary", name)
}

func binaryBaseName(archiveName string) string {
	base := filepath.Base(archiveName)
	base = strings.TrimSuffix(base, ".tar.gz")
	base = strings.TrimSuffix(base, ".zip")
	base = strings.TrimSuffix(base, ".exe")
	return base
}

func currentExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("unable to locate the running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, nil
	}
	return resolved, nil
}

func replaceBinary(binary []byte) (string, error) {
	executable, err := currentExecutable()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(executable)
	temporary, err := os.CreateTemp(directory, ".newhole-update-*")
	if err != nil {
		return "", fmt.Errorf("unable to write next to %s (try again with elevated rights): %w", executable, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if _, err := temporary.Write(binary); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(temporaryName, 0o755); err != nil {
		return "", err
	}

	backup := executable + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(executable, backup); err != nil {
		return "", fmt.Errorf("unable to replace %s: %w", executable, err)
	}
	if err := os.Rename(temporaryName, executable); err != nil {
		_ = os.Rename(backup, executable)
		return "", fmt.Errorf("unable to install the new binary: %w", err)
	}
	_ = os.Remove(backup)
	return executable, nil
}

func Update(desktop bool, checkOnly bool) (*Result, error) {
	published, err := fetchRelease()
	if err != nil {
		return nil, err
	}
	result := &Result{Version: strings.TrimPrefix(published.TagName, "v")}
	if !isNewer(config.Config.Version, result.Version) {
		result.Skipped = fmt.Sprintf("%s is already the newest version", config.Config.Version)
		return result, nil
	}
	if checkOnly {
		result.Skipped = fmt.Sprintf("%s is available (installed: %s)", result.Version, config.Config.Version)
		return result, nil
	}

	asset, err := selectAsset(published, desktop)
	if err != nil {
		return nil, err
	}
	communication.Info("Downloading " + asset.Name)
	payload, err := download(asset)
	if err != nil {
		return nil, err
	}
	binary, err := extract(payload, asset.Name)
	if err != nil {
		return nil, err
	}
	if len(binary) == 0 {
		return nil, errors.New("the downloaded archive is empty")
	}
	path, err := replaceBinary(binary)
	if err != nil {
		return nil, err
	}
	result.Path = path
	result.Updated = true
	return result, nil
}

func isNewer(current string, candidate string) bool {
	currentParts := numericParts(current)
	candidateParts := numericParts(candidate)
	for index := 0; index < len(currentParts) || index < len(candidateParts); index++ {
		currentValue := 0
		if index < len(currentParts) {
			currentValue = currentParts[index]
		}
		candidateValue := 0
		if index < len(candidateParts) {
			candidateValue = candidateParts[index]
		}
		if candidateValue != currentValue {
			return candidateValue > currentValue
		}
	}
	return false
}

func numericParts(version string) []int {
	cleaned := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(version), "v"), "V")
	if index := strings.IndexAny(cleaned, "-+"); index >= 0 {
		cleaned = cleaned[:index]
	}
	parts := []int{}
	for _, part := range strings.Split(cleaned, ".") {
		value := 0
		for _, character := range part {
			if character < '0' || character > '9' {
				break
			}
			value = value*10 + int(character-'0')
		}
		parts = append(parts, value)
	}
	return parts
}
