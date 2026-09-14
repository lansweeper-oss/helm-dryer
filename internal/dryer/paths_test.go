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
		chartPath      string
		sourcePath     string
		expectedRoot   string
		expectedPrefix string
	}{
		"source path is trimmed from the chart path": {
			chartPath:    testChartPath,
			sourcePath:   "charts/app",
			expectedRoot: testRepoRoot,
		},
		"source path may be expressed as absolute": {
			chartPath:    testChartPath,
			sourcePath:   "/charts/app",
			expectedRoot: testRepoRoot,
		},
		"an empty source path means the chart is at the repository root": {
			chartPath:    testRepoRoot,
			sourcePath:   "",
			expectedRoot: testRepoRoot,
		},
		"a dotted source path means the chart is at the repository root": {
			chartPath:    testRepoRoot,
			sourcePath:   ".",
			expectedRoot: testRepoRoot,
		},
		"the repository may be checked out at the filesystem root": {
			chartPath:    "/charts/app",
			sourcePath:   "charts/app",
			expectedRoot: "/",
		},
		"a source path absent from the chart path becomes the trimmed prefix": {
			chartPath:      testChartPath,
			sourcePath:     "charts/other",
			expectedRoot:   testChartPath,
			expectedPrefix: "charts/other",
		},
		"tarball narrowed to the application directory itself": {
			chartPath:      "/tmp/cmp-workdir",
			sourcePath:     "charts/app",
			expectedRoot:   "/tmp/cmp-workdir",
			expectedPrefix: "charts/app",
		},
		"partial match detects trimmed prefix from manifest-generate-paths": {
			chartPath:      "/tmp/cmp/aqua-kube-enforcer",
			sourcePath:     "security/aqua-kube-enforcer",
			expectedRoot:   "/tmp/cmp",
			expectedPrefix: "security",
		},
		"partial match with deeper nesting": {
			chartPath:      "/tmp/cmp/b/app",
			sourcePath:     "a/b/app",
			expectedRoot:   "/tmp/cmp",
			expectedPrefix: "a",
		},
		"partial match single segment source path has no prefix": {
			chartPath:    "/tmp/cmp/app",
			sourcePath:   "app",
			expectedRoot: "/tmp/cmp",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := repoRootFromSourcePath(test.chartPath, test.sourcePath)

			require.NoError(t, err, "The repository root should be resolved")
			assert.Equal(t, test.expectedRoot, result.root, "The repository root is resolved to an unexpected path")
			assert.Equal(t, test.expectedPrefix, result.trimmedPrefix, "The trimmed prefix is unexpected")
		})
	}
}

func TestResolveRepoRootFromEnv(t *testing.T) {
	tests := map[string]struct {
		sourcePath     string
		repoRoot       string
		expected       string
		expectedPrefix string
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
		"a source path absent from the chart path keeps the chart path as root": {
			sourcePath:     "charts/other",
			expected:       testChartPath,
			expectedPrefix: "charts/other",
		},
		"partial match stores trimmed prefix": {
			sourcePath:     "other/app",
			expected:       "/repo/charts",
			expectedPrefix: "other",
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
			assert.Equal(t, test.expectedPrefix, input.repoRootPrefix, "Unexpected trimmed prefix")
		})
	}
}

func TestResolveValuesFileWithPrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		file           string
		repoRoot       string
		repoRootPrefix string
		expected       string
		expectedErr    error
	}{
		"absolute file with matching prefix is stripped": {
			file:           "/security/_values/foo.yaml",
			repoRoot:       "/tmp/cmp",
			repoRootPrefix: "security",
			expected:       "/tmp/cmp/_values/foo.yaml",
		},
		"absolute file without matching prefix resolves normally": {
			file:           "/other/values.yaml",
			repoRoot:       "/tmp/cmp",
			repoRootPrefix: "security",
			expected:       "/tmp/cmp/other/values.yaml",
		},
		"no prefix behaves like the default": {
			file:     "/env/prod/values.yaml",
			repoRoot: testRepoRoot,
			expected: "/repo/env/prod/values.yaml",
		},
		"relative file ignores prefix": {
			file:           "values.yaml",
			repoRoot:       "/tmp/cmp",
			repoRootPrefix: "security",
			expected:       testChartPath + "/values.yaml",
		},
		"full source path prefix strips when the tarball root is the app directory": {
			file:           "/charts/app/values-prod.yaml",
			repoRoot:       "/tmp/cmp-workdir",
			repoRootPrefix: "charts/app",
			expected:       "/tmp/cmp-workdir/values-prod.yaml",
		},
		"prefix stripping preserves escape detection": {
			file:           "/security/../../etc/passwd",
			repoRoot:       "/tmp/cmp",
			repoRootPrefix: "security",
			expectedErr:    dryerr.ErrOutsideRepoRoot,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := &Input{}
			input.Settings.Path = testChartPath
			input.Settings.RepoRoot = test.repoRoot
			input.repoRootPrefix = test.repoRootPrefix

			resolved, err := input.resolveValuesFile(test.file)

			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}
