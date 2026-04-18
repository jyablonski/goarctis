package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jyablonski/goarctis/pkg/version"
)

const (
	githubOwner = "jyablonski"
	githubRepo  = "goarctis"
)

var (
	githubAPI = "https://api.github.com"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// Run checks for the latest release and updates the binary if a newer version
// is available. After a successful update it restarts the goarctis systemd
// user service.
func Run() error {
	currentVersion := version.Version
	if currentVersion == "dev" {
		fmt.Println("skipping self-update: running a dev build")
		return nil
	}

	latestRelease, err := getLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	if compareVersions(currentVersion, latestRelease.TagName) >= 0 {
		fmt.Printf("success: you're on the latest version of goarctis (%s)\n", latestRelease.TagName)
		return nil
	}

	assetName := fmt.Sprintf("goarctis-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, asset := range latestRelease.Assets {
		if asset.Name == assetName {
			downloadURL = asset.DownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if err := downloadAndReplace(execPath, downloadURL); err != nil {
		return fmt.Errorf("failed to update binary: %w", err)
	}

	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", githubOwner, githubRepo, latestRelease.TagName)
	fmt.Printf("success: upgraded goarctis from %s to %s! %s\n", currentVersion, latestRelease.TagName, releaseURL)

	if err := restartService(); err != nil {
		fmt.Printf("warning: failed to restart goarctis.service: %v\n", err)
		fmt.Println("you may need to restart the service manually: systemctl --user restart goarctis.service")
	}

	return nil
}

func getLatestRelease() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPI, githubOwner, githubRepo)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// compareVersions compares two semantic version strings.
// Returns -1 if current < latest, 0 if equal, 1 if current > latest.
func compareVersions(current, latest string) int {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	currentParts := strings.Split(current, ".")
	latestParts := strings.Split(latest, ".")

	maxLen := len(currentParts)
	if len(latestParts) > maxLen {
		maxLen = len(latestParts)
	}

	for i := 0; i < maxLen; i++ {
		var currentPart, latestPart int
		if i < len(currentParts) {
			fmt.Sscanf(currentParts[i], "%d", &currentPart)
		}
		if i < len(latestParts) {
			fmt.Sscanf(latestParts[i], "%d", &latestPart)
		}

		if currentPart < latestPart {
			return -1
		}
		if currentPart > latestPart {
			return 1
		}
	}

	return 0
}

// downloadAndReplace downloads the new binary from downloadURL and replaces
// the running binary at execPath. It writes to a temporary file adjacent to
// the target first so that os.Rename is an atomic same-filesystem operation.
// If the rename fails (e.g. cross-filesystem), it falls back to a copy.
func downloadAndReplace(execPath, downloadURL string) error {
	// Write temp file in the same directory as the target binary so that
	// os.Rename stays on the same filesystem (atomic).
	tempFile := execPath + ".tmp"

	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tempFile)
		return err
	}

	if err := out.Close(); err != nil {
		os.Remove(tempFile)
		return err
	}

	if err := os.Chmod(tempFile, 0755); err != nil {
		os.Remove(tempFile)
		return err
	}

	if err := os.Rename(tempFile, execPath); err != nil {
		if copyErr := crossFSReplace(tempFile, execPath); copyErr != nil {
			os.Remove(tempFile)
			return fmt.Errorf("rename failed (%v) and cross-fs copy also failed: %w", err, copyErr)
		}
		os.Remove(tempFile)
	}

	return nil
}

// crossFSReplace copies src to dst when they live on different filesystems.
func crossFSReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	return os.Chmod(dst, 0755)
}

// restartService restarts the goarctis systemd user service.
func restartService() error {
	cmd := exec.Command("systemctl", "--user", "restart", "goarctis.service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
