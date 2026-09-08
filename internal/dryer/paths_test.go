package dryer //nolint:testpackage // tests unexported path helpers

import (
	"path/filepath"
	"testing"

	"github.com/lansweeper-oss/helm-dryer/internal/argo"
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testChartPath = "/repo/charts/app"
	testRepoRoot  = "/repo"
)

func TestResolveValuesFile(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		file        string
		expected    string
		expectedErr error
	}{
		"relative file resolves from the chart path": {
			file:     "values.yaml",
			expected: filepath.Join(testChartPath, "values.yaml"),
		},
		"relative file may point above the chart path": {
			file:     "../common/values.yaml",
			expected: "/repo/charts/common/values.yaml",
		},
		"absolute file resolves from the repository root": {
			file:     "/env/prod/values.yaml",
			expected: "/repo/env/prod/values.yaml",
		},
		"absolute file is cleaned while inside the repository root": {
			file:     "/env/../env/prod/values.yaml",
			expected: "/repo/env/prod/values.yaml",
		},
		"absolute file cannot escape the repository root": {
			file:        "/../../etc/passwd",
			expectedErr: dryerr.ErrOutsideRepoRoot,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := &Input{}
			input.Settings.Path = testChartPath
			input.Settings.RepoRoot = testRepoRoot

			resolved, err := input.resolveValuesFile(test.file)

			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr, "Escaping the repository root should be rejected")

				return
			}

			require.NoError(t, err, "The values file should be resolved")
			assert.Equal(t, test.expected, resolved, "The values file is resolved to an unexpected path")
		})
	}
}

func TestRepoRootFromSourcePath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		chartPath   string
		sourcePath  string
		expected    string
		expectedErr error
	}{
		"source path is trimmed from the chart path": {
			chartPath:  testChartPath,
			sourcePath: "charts/app",
			expected:   testRepoRoot,
		},
		"source path may be expressed as absolute": {
			chartPath:  testChartPath,
			sourcePath: "/charts/app",
			expected:   testRepoRoot,
		},
		"an empty source path means the chart is at the repository root": {
			chartPath:  testRepoRoot,
			sourcePath: "",
			expected:   testRepoRoot,
		},
		"a dotted source path means the chart is at the repository root": {
			chartPath:  testRepoRoot,
			sourcePath: ".",
			expected:   testRepoRoot,
		},
		"the repository may be checked out at the filesystem root": {
			chartPath:  "/charts/app",
			sourcePath: "charts/app",
			expected:   "/",
		},
		"a source path not matching the chart path is an error": {
			chartPath:   testChartPath,
			sourcePath:  "charts/other",
			expectedErr: dryerr.ErrSourcePathMismatch,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root, err := repoRootFromSourcePath(test.chartPath, test.sourcePath)

			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr, "A mismatching source path should be reported")

				return
			}

			require.NoError(t, err, "The repository root should be resolved")
			assert.Equal(t, test.expected, root, "The repository root is resolved to an unexpected path")
		})
	}
}

func TestResolveRepoRootFromEnv(t *testing.T) {
	tests := map[string]struct {
		sourcePath string
		repoRoot   string
		expected   string
	}{
		"the root is derived from the environment": {
			sourcePath: "charts/app",
			expected:   testRepoRoot,
		},
		"an explicit root is not overridden": {
			sourcePath: "charts/app",
			repoRoot:   "/elsewhere",
			expected:   "/elsewhere",
		},
		"an undetermined root falls back to the chart path": {
			sourcePath: "charts/other",
			expected:   testChartPath,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv(argo.SourcePath, test.sourcePath)

			input := &Input{}
			input.Settings.Path = testChartPath
			input.Settings.RepoRoot = test.repoRoot

			input.resolveRepoRootFromEnv()

			assert.Equal(t, test.expected, input.repoRoot(), "Unexpected base to resolve absolute values files")
		})
	}
}
