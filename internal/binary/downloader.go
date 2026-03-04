package binary

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BinDir returns the directory where downloaded binaries are stored (~/.foreman/bin/).
func BinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".foreman", "bin"), nil
}

// parseGitHubReleaseURL parses a GitHub releases tag URL and returns owner, repo, version.
// Expected format: https://github.com/{owner}/{repo}/releases/tag/{version}
func parseGitHubReleaseURL(url string) (owner, repo, version string, err error) {
	// Strip trailing slash
	url = strings.TrimRight(url, "/")

	// Find the github.com part and parse path segments
	const prefix = "github.com/"
	idx := strings.Index(url, prefix)
	if idx == -1 {
		return "", "", "", fmt.Errorf("not a valid GitHub URL: %s", url)
	}

	path := url[idx+len(prefix):]
	parts := strings.Split(path, "/")
	// Expected: owner/repo/releases/tag/version
	if len(parts) < 5 || parts[2] != "releases" || parts[3] != "tag" {
		return "", "", "", fmt.Errorf("not a valid GitHub releases tag URL: %s", url)
	}

	return parts[0], parts[1], parts[4], nil
}

// buildDownloadURL constructs the binary download URL for the current OS and architecture.
// binaryName is the base name of the binary (e.g., "mdviewer"). If empty, defaults to repo name.
// Format: https://github.com/{owner}/{repo}/releases/download/{version}/{binaryName}-{os}-{arch}[.exe]
func buildDownloadURL(owner, repo, version, binaryName string) string {
	if binaryName == "" {
		binaryName = repo
	}
	osName := runtime.GOOS
	arch := runtime.GOARCH

	assetName := fmt.Sprintf("%s-%s-%s", binaryName, osName, arch)
	if osName == "windows" {
		assetName += ".exe"
	}

	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, version, assetName)
}

// localFileName returns a unique local file name for a downloaded binary.
func localFileName(binaryName, version string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	name := fmt.Sprintf("%s-%s-%s-%s", binaryName, version, osName, arch)
	if osName == "windows" {
		name += ".exe"
	}
	return name
}

// EnsureBinary checks if the binary for the given source URL exists locally.
// If not, it downloads it. binaryName is the base name of the release asset
// (e.g., "mdviewer"); if empty, defaults to the repo name.
// Returns the absolute path to the binary.
func EnsureBinary(sourceURL, binaryName string) (string, error) {
	owner, repo, version, err := parseGitHubReleaseURL(sourceURL)
	if err != nil {
		return "", err
	}

	if binaryName == "" {
		binaryName = repo
	}

	binDir, err := BinDir()
	if err != nil {
		return "", err
	}

	localName := localFileName(binaryName, version)
	localPath := filepath.Join(binDir, localName)

	// Check if binary already exists
	if _, err := os.Stat(localPath); err == nil {
		log.Printf("[binary] found existing binary: %s", localPath)
		return localPath, nil
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("creating bin directory: %w", err)
	}

	downloadURL := buildDownloadURL(owner, repo, version, binaryName)
	log.Printf("[binary] downloading %s -> %s", downloadURL, localPath)

	if err := downloadFile(downloadURL, localPath); err != nil {
		return "", fmt.Errorf("downloading binary: %w", err)
	}

	// Make binary executable
	if err := os.Chmod(localPath, 0755); err != nil {
		return "", fmt.Errorf("making binary executable: %w", err)
	}

	log.Printf("[binary] successfully downloaded binary: %s", localPath)
	return localPath, nil
}

// downloadFile downloads a URL to a local file path.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d for URL: %s", resp.StatusCode, url)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		// Clean up partial file on error
		os.Remove(destPath)
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}
