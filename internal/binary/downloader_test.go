package binary

import (
	"testing"
)

func TestParseGitHubReleaseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		owner   string
		repo    string
		version string
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://github.com/roraja/foreman/releases/tag/v0.0.2",
			owner:   "roraja",
			repo:    "foreman",
			version: "v0.0.2",
		},
		{
			name:    "valid URL with trailing slash",
			url:     "https://github.com/roraja/foreman/releases/tag/v0.0.2/",
			owner:   "roraja",
			repo:    "foreman",
			version: "v0.0.2",
		},
		{
			name:    "different owner and repo",
			url:     "https://github.com/myorg/myapp/releases/tag/v1.2.3",
			owner:   "myorg",
			repo:    "myapp",
			version: "v1.2.3",
		},
		{
			name:    "not a GitHub URL",
			url:     "https://gitlab.com/user/repo/releases/tag/v1.0",
			wantErr: true,
		},
		{
			name:    "not a releases tag URL",
			url:     "https://github.com/user/repo/tree/main",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, version, err := parseGitHubReleaseURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.owner {
				t.Errorf("owner = %q, want %q", owner, tt.owner)
			}
			if repo != tt.repo {
				t.Errorf("repo = %q, want %q", repo, tt.repo)
			}
			if version != tt.version {
				t.Errorf("version = %q, want %q", version, tt.version)
			}
		})
	}
}

func TestBuildDownloadURL(t *testing.T) {
	// With explicit binary name
	url := buildDownloadURL("roraja", "markdown-go", "v1.0.1", "mdviewer")
	expected := "https://github.com/roraja/markdown-go/releases/download/v1.0.1/mdviewer-"
	if len(url) < len(expected) || url[:len(expected)] != expected {
		t.Errorf("URL = %q, expected prefix %q", url, expected)
	}

	// Without binary name (defaults to repo)
	url2 := buildDownloadURL("roraja", "foreman", "v0.0.2", "")
	expected2 := "https://github.com/roraja/foreman/releases/download/v0.0.2/foreman-"
	if len(url2) < len(expected2) || url2[:len(expected2)] != expected2 {
		t.Errorf("URL = %q, expected prefix %q", url2, expected2)
	}
}

func TestLocalFileName(t *testing.T) {
	name := localFileName("mdviewer", "v1.0.1")

	if name == "" {
		t.Error("expected non-empty file name")
	}
	expected := "mdviewer-v1.0.1-"
	if len(name) < len(expected) || name[:len(expected)] != expected {
		t.Errorf("name = %q, expected prefix %q", name, expected)
	}
}
