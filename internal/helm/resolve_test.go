package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	ociRegistry "helm.sh/helm/v3/pkg/registry"
)

func TestFindBestVersionMatch(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name              string
		availableVersions []string
		constraint        string
		expectedVersion   string
		expectedError     bool
	}{
		{
			name:              "exact version match",
			availableVersions: []string{"1.0.0", "1.1.0", "2.0.0"},
			constraint:        "1.0.0",
			expectedVersion:   "1.0.0",
			expectedError:     false,
		},
		{
			name:              "greater than constraint",
			availableVersions: []string{"0.9.0", "1.0.0", "1.1.0", "2.0.0"},
			constraint:        ">1.0.0",
			expectedVersion:   "2.0.0",
			expectedError:     false,
		},
		{
			name:              "tilde constraint - returns latest patch",
			availableVersions: []string{"1.2.0", "1.2.1", "1.2.5", "1.3.0", "2.0.0"},
			constraint:        "~1.2.0",
			expectedVersion:   "1.2.5",
			expectedError:     false,
		},
		{
			name:              "caret constraint - returns latest minor",
			availableVersions: []string{"1.2.0", "1.5.0", "2.0.0", "2.1.0"},
			constraint:        "^1.2.0",
			expectedVersion:   "1.5.0",
			expectedError:     false,
		},
		{
			name:              "range constraint",
			availableVersions: []string{"0.9.0", "1.0.0", "1.5.0", "2.0.0"},
			constraint:        ">=1.0.0,<2.0.0",
			expectedVersion:   "1.5.0",
			expectedError:     false,
		},
		{
			name:              "skips non-semver tags",
			availableVersions: []string{"latest", "v1.0.0", "1.0.0", "1.1.0", "main"},
			constraint:        ">=1.0.0",
			expectedVersion:   "1.1.0",
			expectedError:     false,
		},
		{
			name:              "no matching version",
			availableVersions: []string{"1.0.0", "1.1.0"},
			constraint:        "2.0.0",
			expectedError:     true,
		},
		{
			name:              "invalid constraint",
			availableVersions: []string{"1.0.0"},
			constraint:        "invalid-constraint",
			expectedError:     true,
		},
		{
			name:              "empty versions list",
			availableVersions: []string{},
			constraint:        "1.0.0",
			expectedError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := client.findBestVersionMatch(tt.availableVersions, tt.constraint)

			if tt.expectedError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectedError && version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}

func TestResolveHTTPVersion(t *testing.T) {
	// Create a mock HTTP server that returns a valid Helm index
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "index.yaml") {
			w.Header().Set("Content-Type", "application/yaml")
			// Valid Helm index format (YAML, no "kind" field)
			fmt.Fprint(w, `apiVersion: v1
entries:
  mychart:
    - name: mychart
      version: 1.0.0
      appVersion: 1.0.0
      created: 2024-01-01T00:00:00Z
      digest: abc123
      urls:
        - https://example.com/mychart-1.0.0.tgz
    - name: mychart
      version: 1.1.0
      appVersion: 1.1.0
      created: 2024-01-02T00:00:00Z
      digest: def456
      urls:
        - https://example.com/mychart-1.1.0.tgz
    - name: mychart
      version: 2.0.0
      appVersion: 2.0.0
      created: 2024-01-03T00:00:00Z
      digest: ghi789
      urls:
        - https://example.com/mychart-2.0.0.tgz
`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &Client{
		Debug: false,
	}

	tests := []struct {
		name            string
		dep             *chart.Dependency
		expectedVersion string
		expectedError   bool
	}{
		{
			name: "resolve exact version",
			dep: &chart.Dependency{
				Name:       "mychart",
				Version:    "1.0.0",
				Repository: server.URL,
			},
			expectedVersion: "1.0.0",
			expectedError:   false,
		},
		{
			name: "resolve version with constraint",
			dep: &chart.Dependency{
				Name:       "mychart",
				Version:    ">=1.0.0,<2.0.0",
				Repository: server.URL,
			},
			expectedVersion: "1.1.0",
			expectedError:   false,
		},
		{
			name: "resolve latest version",
			dep: &chart.Dependency{
				Name:       "mychart",
				Version:    ">=0.0.0",
				Repository: server.URL,
			},
			expectedVersion: "2.0.0",
			expectedError:   false,
		},
		{
			name: "chart not found",
			dep: &chart.Dependency{
				Name:       "nonexistent",
				Version:    "1.0.0",
				Repository: server.URL,
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := client.resolveHTTPVersion(tt.dep)

			if tt.expectedError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectedError && version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}

func TestResolveLocalVersion(t *testing.T) {
	tests := []struct {
		name            string
		chartVersion    string
		constraint      string
		expectedVersion string
		expectedError   bool
	}{
		{
			name:            "local chart version satisfies constraint",
			chartVersion:    "1.2.0",
			constraint:      ">=1.0.0",
			expectedVersion: "1.2.0",
			expectedError:   false,
		},
		{
			name:          "local chart version violates constraint",
			chartVersion:  "0.9.0",
			constraint:    ">=1.0.0",
			expectedError: true,
		},
		{
			name:            "local chart exact version match",
			chartVersion:    "1.0.0",
			constraint:      "1.0.0",
			expectedVersion: "1.0.0",
			expectedError:   false,
		},
		{
			name:            "local chart with tilde constraint",
			chartVersion:    "1.2.3",
			constraint:      "~1.2.0",
			expectedVersion: "1.2.3",
			expectedError:   false,
		},
		{
			name:          "local chart invalid version string",
			chartVersion:  "not-a-version",
			constraint:    ">=1.0.0",
			expectedError: true,
		},
		{
			name:          "local chart invalid constraint",
			chartVersion:  "1.0.0",
			constraint:    "invalid-constraint",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary chart directory
			tmpDir := t.TempDir()
			chartFile := filepath.Join(tmpDir, "Chart.yaml")

			// Create a valid Chart.yaml
			chartContent := fmt.Sprintf(`apiVersion: v2
name: testchart
version: %s
description: Test chart
`, tt.chartVersion)

			err := os.WriteFile(chartFile, []byte(chartContent), 0o644)
			if err != nil {
				t.Fatalf("failed to create test chart: %v", err)
			}

			client := &Client{
				Path: t.TempDir(), // Parent path for resolving relative paths
			}

			dep := &chart.Dependency{
				Name:       "testchart",
				Version:    tt.constraint,
				Repository: LocalRepoPrefix + tmpDir,
			}

			version, err := client.resolveLocalVersion(dep)

			if tt.expectedError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectedError && version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		isOCI      bool
	}{
		{
			name:       "HTTP repository",
			repository: "https://example.com/charts",
			isOCI:      false,
		},
		{
			name:       "HTTPS repository",
			repository: "https://example.com/charts",
			isOCI:      false,
		},
		{
			name:       "OCI repository with oci:// prefix",
			repository: "oci://registry.example.com/charts",
			isOCI:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := &chart.Dependency{
				Name:       "test-chart",
				Version:    "1.0.0",
				Repository: tt.repository,
			}

			// Verify the dispatcher logic works correctly
			isOCI := ociRegistry.IsOCI(dep.Repository)
			if isOCI != tt.isOCI {
				t.Errorf("expected isOCI=%v, got %v for repository %s", tt.isOCI, isOCI, tt.repository)
			}
		})
	}
}

func TestFindBestVersionMatchWithOCITags(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name              string
		availableVersions []string
		constraint        string
		expectedVersion   string
		expectedError     bool
	}{
		{
			name:              "OCI tags with v prefix",
			availableVersions: []string{"v1.0.0", "v1.1.0", "v2.0.0"},
			constraint:        ">=1.0.0",
			expectedVersion:   "v2.0.0",
			expectedError:     false, // semver handles v prefix
		},
		{
			name:              "OCI tags without v prefix",
			availableVersions: []string{"1.0.0", "1.1.0", "2.0.0"},
			constraint:        ">=1.0.0",
			expectedVersion:   "2.0.0",
			expectedError:     false,
		},
		{
			name:              "OCI mixed valid and invalid tags",
			availableVersions: []string{"latest", "main", "1.0.0", "1.1.0", "stable", "2.0.0"},
			constraint:        ">=1.0.0,<2.0.0",
			expectedVersion:   "1.1.0",
			expectedError:     false,
		},
		{
			name:              "OCI tags with build metadata",
			availableVersions: []string{"1.0.0", "1.0.0+build1", "1.0.1"},
			constraint:        "1.0.0",
			expectedVersion:   "1.0.0",
			expectedError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := client.findBestVersionMatch(tt.availableVersions, tt.constraint)

			if tt.expectedError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectedError && version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}
