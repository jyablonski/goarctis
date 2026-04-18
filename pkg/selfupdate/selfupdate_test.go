package selfupdate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected int
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.2.0", 0},
		{"v0.3.0", "v0.2.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.9.9", "v1.0.0", -1},
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.2.4", "v1.2.3", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v0.0.1", "v0.0.2", -1},
		{"v1.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0", 0},
		{"v1.0", "v1.0.1", -1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.current, tt.latest), func(t *testing.T) {
			result := compareVersions(tt.current, tt.latest)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, result, tt.expected)
			}
		})
	}
}

func TestDownloadAndReplace(t *testing.T) {
	fakeContent := []byte("#!/bin/sh\necho new-version\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(fakeContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "goarctis")

	if err := os.WriteFile(execPath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	if err := downloadAndReplace(execPath, server.URL+"/goarctis-linux-amd64"); err != nil {
		t.Fatalf("downloadAndReplace failed: %v", err)
	}

	data, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}

	if string(data) != string(fakeContent) {
		t.Errorf("binary content mismatch: got %q, want %q", string(data), string(fakeContent))
	}

	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("failed to stat replaced binary: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("replaced binary is not executable")
	}
}

func TestDownloadAndReplaceHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "goarctis")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	err := downloadAndReplace(execPath, server.URL+"/missing")
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

func TestGetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"tag_name": "v0.3.0",
			"assets": [
				{
					"name": "goarctis-linux-amd64",
					"browser_download_url": "https://example.com/goarctis-linux-amd64"
				}
			]
		}`)
	}))
	defer server.Close()

	originalAPI := githubAPI
	githubAPI = server.URL
	defer func() { githubAPI = originalAPI }()

	release, err := getLatestRelease()
	if err != nil {
		t.Fatalf("getLatestRelease failed: %v", err)
	}

	if release.TagName != "v0.3.0" {
		t.Errorf("expected tag v0.3.0, got %s", release.TagName)
	}

	if len(release.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(release.Assets))
	}

	if release.Assets[0].Name != "goarctis-linux-amd64" {
		t.Errorf("expected asset name goarctis-linux-amd64, got %s", release.Assets[0].Name)
	}
}

func TestGetLatestReleaseAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalAPI := githubAPI
	githubAPI = server.URL
	defer func() { githubAPI = originalAPI }()

	_, err := getLatestRelease()
	if err == nil {
		t.Fatal("expected error for API 500, got nil")
	}
}

func TestCrossFSReplace(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src-binary")
	dst := filepath.Join(tmpDir, "dst-binary")

	srcContent := []byte("new-binary-content")
	if err := os.WriteFile(src, srcContent, 0644); err != nil {
		t.Fatalf("failed to write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old-binary-content"), 0644); err != nil {
		t.Fatalf("failed to write dst: %v", err)
	}

	if err := crossFSReplace(src, dst); err != nil {
		t.Fatalf("crossFSReplace failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dst: %v", err)
	}

	if string(data) != string(srcContent) {
		t.Errorf("dst content mismatch: got %q, want %q", string(data), string(srcContent))
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("failed to stat dst: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("dst is not executable after crossFSReplace")
	}
}
