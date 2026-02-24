package client_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lansweeper/helm-dryer/internal/cli"
	client "github.com/lansweeper/helm-dryer/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func TestDependenciesNeedUpdate(t *testing.T) {
	t.Parallel()

	// Create a temporary directory to simulate a chart path
	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	// Create a dummy Chart.yaml file with dependencies
	chartFile := filepath.Join(tempDir, "Chart.yaml")
	err := os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: redis
    version: 6.0.0
`), 0o644)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Test case: DependenciesNeedUpdate returns true when dependency is missing
	helmClient := client.Client{Path: tempDir, Debug: true}
	err = helmClient.LoadChart()
	require.NoError(t, err, "Failed to load chart")

	needsUpdate := helmClient.DependenciesNeedUpdate()
	assert.True(t, needsUpdate, "DependenciesNeedUpdate should return true when dependency is missing")

	// Create a dummy dependency file to simulate an existing dependency
	chartsDir := filepath.Join(tempDir, "charts")
	err = os.Mkdir(chartsDir, 0o700)
	require.NoError(t, err, "Failed to create dependencies directory")

	dependencyFile := filepath.Join(chartsDir, "redis-6.0.0.tgz")
	err = os.WriteFile(dependencyFile, []byte("dummy content"), 0o644)
	require.NoError(t, err, "Failed to create dependency file")

	// Test case: DependenciesNeedUpdate returns true when TTL is set to zero
	needsUpdate = helmClient.DependenciesNeedUpdate()
	assert.True(t, needsUpdate, "DependenciesNeedUpdate should return true when dependencies are expired")

	// Test case: DependenciesNeedUpdate returns false when all dependencies exists and TTL is not expired.
	// h.TTL field represents a cutoff time, files modified after this time are considered valid.
	helmClient.TTL = time.Now().Add(-10 * time.Minute)
	needsUpdate = helmClient.DependenciesNeedUpdate()
	assert.False(t, needsUpdate, "DependenciesNeedUpdate should return false when the TTL is not expired")
}

func TestEnsureCacheDirs(t *testing.T) {
	// Test case: EnsureCacheDir creates the cache directory if it doesn't exist
	cacheDir := filepath.Join(os.TempDir(), "helm-cache-test")
	defer os.RemoveAll(cacheDir)

	// Set the HELM_CACHE_HOME environment variable to the test cache directory
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	err := client.EnsureCacheDirs(cacheDir)
	require.NoError(t, err, "EnsureCacheDirs should not return an error")

	// Verify the cache directory was created
	_, err = os.Stat(cacheDir)
	require.NoError(t, err, "Cache directory should exist")

	// Test case: EnsureCacheDir does not return an error if the cache directory already exists
	err = client.EnsureCacheDirs(cacheDir)
	require.NoError(t, err, "EnsureCacheDirs should not return an error if the directory already exists")

	// Test case: EnsureCacheDir returns an error if the cache directory cannot be created
	invalidDir := "/invalid-path/helm-cache"
	t.Setenv("HELM_CACHE_HOME", invalidDir)

	err = client.EnsureCacheDirs(cacheDir)
	require.Error(t, err, "EnsureCacheDirs should return an error for an invalid cache directory")
}

func TestUpdateDeps(t *testing.T) {
	t.Parallel()

	const (
		chartRepository = "http://charts.gitlab.io/"
		chartName       = "gitlab"
		chartVersion    = "8.11.2"
	)
	// Create a temporary directory to simulate a chart path
	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	// Create a dummy Chart.yaml file to simulate a Helm chart
	chartFile := filepath.Join(tempDir, "Chart.yaml")
	err := os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart-with-dependencies
version: 0.1.0
dependencies:
  - name: `+chartName+`
    version: `+chartVersion+`
    repository: `+chartRepository+`
`), 0o644)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Create a dummy charts directory to simulate dependencies
	chartsDir := filepath.Join(tempDir, "charts")
	err = os.Mkdir(chartsDir, 0o750)
	require.NoError(t, err, "Failed to create charts directory")

	// Test case: UpdateDeps with a valid chart path
	helmClient := client.Client{
		Credentials: &cli.Credentials{},
		Debug:       true,
		Path:        tempDir,
	}
	_, err = helmClient.ReadChartDependencies()
	require.NoError(t, err, "ReadChartDependencies should not return an error")
	// Verify that the gitlab dependency file exists
	archiveFile := client.GetArchiveName(chartName, chartVersion)
	dependencyFile := filepath.Join(chartsDir, archiveFile)
	fileInfo, err := os.Stat(dependencyFile)
	require.NoError(t, err, archiveFile+" should exist in the charts directory")

	// Test case: Old dependency gets ignored and is downloaded again
	helmClient.TTL = time.Time{}

	// Call ReadChartDependencies again to simulate updating dependencies
	_, err = helmClient.ReadChartDependencies()
	require.NoError(t, err, "ReadChartDependencies should not return an error")

	// Get the modification time of the dependency file after updating
	newFileInfo, err := os.Stat(dependencyFile)
	require.NoError(t, err, "Failed to stat updated dependency file")

	// Verify that the modification time has changed
	assert.NotEqual(
		t,
		fileInfo.ModTime(),
		newFileInfo.ModTime(),
		"Modification time should change after updating dependencies",
	)

	// Test case: Invalid chart path
	helmClient.Path = "invalid-path"
	_, err = helmClient.ReadChartDependencies()
	require.Error(t, err, "ReadChartDependencies should return an error for an invalid chart path")
}

func TestStoreDeps(t *testing.T) {
	const (
		chartRepository = "https://charts.jetstack.io"
		chartName       = "cert-manager"
		chartVersion    = "1.18.2"
	)
	// Create a temporary directory to simulate a chart path
	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	// Create a temporary cache directory
	cacheDir := t.TempDir()
	defer os.RemoveAll(cacheDir)

	// Set the HELM_CACHE_HOME environment variable to the test cache directory
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	// Create a dummy Chart.yaml file with dependencies
	chartFile := filepath.Join(tempDir, "Chart.yaml")
	err := os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: `+chartName+`
    repository: `+chartRepository+`
    name: cert-manager
    version: `+chartVersion+`
    alias: test
`), 0o644)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Create charts directory and dependency files
	chartsDir := filepath.Join(tempDir, "charts")
	err = os.Mkdir(chartsDir, 0o755)
	require.NoError(t, err, "Failed to create charts directory")

	helmClient := client.Client{Path: tempDir, Debug: true}
	// Downloaded file contains v in the version for this chart :(
	archiveFile := fmt.Sprintf("%s-v%s.tgz", chartName, chartVersion)

	err = helmClient.UpdateDeps()
	require.NoError(t, err, "UpdateDeps should not return an error")

	// Verify that the dependency file was retrieved
	archivedChart := filepath.Join(chartsDir, archiveFile)
	_, err = os.Stat(archivedChart)
	require.NoError(t, err, archiveFile+" should exist in the charts directory")

	// Test case: StoreDeps successfully copies dependency files to cache
	err = helmClient.LoadChart()
	require.NoError(t, err, "Failed to load chart")

	err = client.EnsureCacheDirs(tempDir)
	require.NoError(t, err, "Failed to ensure cache directories")

	// Verify that dependency files were copied to cache
	helmClient.CacheDependencies()

	cachedChart := filepath.Join(cacheDir, "charts", archiveFile)
	_, err = os.Stat(cachedChart)
	require.NoError(t, err, archiveFile+" should exist in cache")
	_, err = os.ReadFile(cachedChart)
	require.NoError(t, err, "Failed to read cached test file")

	// Test case: StoreDeps handles missing dependency files gracefully
	err = os.Remove(archivedChart)
	require.NoError(t, err, "Failed to remove test dependency file")

	err = helmClient.CacheDependencies()
	assert.Contains(t, err.Error(), fmt.Sprintf("failed to copy chart %s to cache directory", archiveFile))

	// Test case: StoreDeps with chart that has no dependencies
	err = os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart-no-deps
version: 0.1.0
`), 0o644)
	require.NoError(t, err, "Failed to create Chart.yaml without dependencies")

	err = helmClient.LoadChart()
	require.NoError(t, err, "Failed to load chart without dependencies")

	err = helmClient.CacheDependencies()
	assert.NoError(t, err, "StoreDeps should not return an error for a chart without dependencies")
}

func TestReadChartDependenciesReloadsAfterUpdate(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sub-chart with its own Chart.yaml and values.yaml
	subChartDir := filepath.Join(tempDir, "subchart")
	err := os.MkdirAll(subChartDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: subchart
version: 0.1.0
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "values.yaml"), []byte(`replicaCount: 3
`), 0o644)
	require.NoError(t, err)

	// Create the parent chart referencing the sub-chart via file://
	parentDir := filepath.Join(tempDir, "parent")
	err = os.MkdirAll(parentDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(parentDir, "Chart.yaml"), []byte(`apiVersion: v2
name: parent-chart
version: 0.1.0
dependencies:
  - name: subchart
    version: 0.1.0
    repository: file://../subchart
`), 0o644)
	require.NoError(t, err)

	// charts/ directory is intentionally absent — EnsureCacheDirs will create it.
	// This simulates a fresh clone where charts/ starts empty.
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	helmClient := client.Client{
		Path:  parentDir,
		Debug: true,
	}

	vals, err := helmClient.ReadChartDependencies()
	require.NoError(t, err)

	// Without the reload after UpdateDeps, Dependencies() would be empty
	// and we would get no values back.
	subchartVals, ok := vals["subchart"]
	require.True(t, ok, "expected subchart values to be present")

	subchartMap, ok := subchartVals.(map[string]any)
	require.True(t, ok, "expected subchart values to be a map")

	assert.InEpsilon(t, float64(3), subchartMap["replicaCount"], 0.01)
}

func TestHasDependencies(t *testing.T) {
	t.Parallel()

	// nil chart returns false
	helmClient := client.Client{}
	assert.False(t, helmClient.HasDependencies(), "nil chart should return false")

	// chart with nil metadata returns false
	helmClient.Chart = &chart.Chart{}
	assert.False(t, helmClient.HasDependencies(), "nil metadata should return false")

	// chart with no dependencies returns false
	helmClient.Chart.Metadata = &chart.Metadata{
		Name:    "test",
		Version: "0.1.0",
	}
	assert.False(t, helmClient.HasDependencies(), "no dependencies should return false")

	// chart with dependencies returns true
	helmClient.Chart.Metadata.Dependencies = []*chart.Dependency{
		{Name: "redis", Version: "6.0.0", Repository: "https://charts.bitnami.com"},
	}
	assert.True(t, helmClient.HasDependencies(), "chart with dependencies should return true")
}

func TestDeduplicateDependencies(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sub-chart
	subChartDir := filepath.Join(tempDir, "subchart")
	err := os.MkdirAll(subChartDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: subchart
version: 0.1.0
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "values.yaml"), []byte(`enabled: true
`), 0o644)
	require.NoError(t, err)

	// Parent chart with the same dependency listed twice
	parentDir := filepath.Join(tempDir, "parent")
	err = os.MkdirAll(parentDir, 0o755)
	require.NoError(t, err)

	// Use an alias on the second entry so Helm validation passes (different
	// effective names), but the dedup key (name|version|repository) matches.
	err = os.WriteFile(filepath.Join(parentDir, "Chart.yaml"), []byte(`apiVersion: v2
name: parent-chart
version: 0.1.0
dependencies:
  - name: subchart
    version: 0.1.0
    repository: file://../subchart
  - name: subchart
    version: 0.1.0
    repository: file://../subchart
    alias: subchart-dup
`), 0o644)
	require.NoError(t, err)

	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	helmClient := client.Client{
		Path:  parentDir,
		Debug: true,
	}

	vals, err := helmClient.ReadChartDependencies()
	require.NoError(t, err)

	// Dedup removes the aliased duplicate; only the original entry remains.
	subchartVals, ok := vals["subchart"]
	require.True(t, ok, "expected subchart values to be present")

	subchartMap, ok := subchartVals.(map[string]any)
	require.True(t, ok, "expected subchart values to be a map")

	assert.Equal(t, true, subchartMap["enabled"])

	// The duplicate (alias "subchart-dup") should have been removed by dedup,
	// so only one entry should exist in the values map.
	assert.Len(t, vals, 1, "dedup should have removed the duplicate dependency")
}
