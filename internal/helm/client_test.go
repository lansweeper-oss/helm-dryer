package client_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lansweeper-oss/helm-dryer/internal/cli"
	client "github.com/lansweeper-oss/helm-dryer/internal/helm"
	utils "github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func TestStaleDependencies(t *testing.T) {
	t.Parallel()

	// Create a temporary directory to simulate a chart path
	tempDir := t.TempDir()

	// Create a dummy Chart.yaml file with dependencies
	chartFile := filepath.Join(tempDir, "Chart.yaml")
	err := os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: redis
    version: 6.0.0
`), utils.ReadWrite)
	require.NoError(t, err, "Failed to create Chart.yaml")

	helmClient := client.Client{Path: tempDir, Debug: true}
	err = helmClient.LoadChart()
	require.NoError(t, err, "Failed to load chart")

	// Test case: returns the missing dependency when archive is absent
	staleDeps := helmClient.StaleDependencies()
	require.Len(t, staleDeps, 1, "should return the missing dependency")
	assert.Equal(t, "redis", staleDeps[0].Name)

	// Create a dummy dependency file to simulate an existing dependency
	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err, "Failed to create dependencies directory")

	dependencyFile := filepath.Join(chartsDir, "redis-6.0.0.tgz")
	err = os.WriteFile(dependencyFile, []byte("dummy content"), utils.ReadWrite)
	require.NoError(t, err, "Failed to create dependency file")

	// Test case: returns all deps when TTL is zero (caching disabled)
	staleDeps = helmClient.StaleDependencies()
	assert.Len(t, staleDeps, 1, "should return all deps when TTL is zero (caching disabled)")

	// Test case: returns empty slice when all dependencies exist and TTL is not expired.
	// h.TTL field represents a cutoff time, files modified after this time are considered valid.
	helmClient.TTL = time.Now().Add(-10 * time.Minute)
	staleDeps = helmClient.StaleDependencies()
	assert.Empty(t, staleDeps, "should return empty slice when all deps are fresh")
}

func TestUpdateDepsFileProtocol(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	// Create two sub-charts
	for _, sub := range []struct{ name, version, value string }{
		{"subchart-a", "0.1.0", "valueA: true"},
		{"subchart-b", "0.2.0", "valueB: 42"},
	} {
		subDir := filepath.Join(tempDir, sub.name)
		err := os.MkdirAll(subDir, utils.ReadWriteDir)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subDir, "Chart.yaml"), fmt.Appendf(nil,
			"apiVersion: v2\nname: %s\nversion: %s\n", sub.name, sub.version), utils.ReadWrite)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subDir, "values.yaml"), []byte(sub.value+"\n"), utils.ReadWrite)
		require.NoError(t, err)
	}

	// Create parent chart depending on both sub-charts
	parentDir := filepath.Join(tempDir, "parent")
	err := os.MkdirAll(parentDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(parentDir, "Chart.yaml"), []byte(`apiVersion: v2
name: parent
version: 0.1.0
dependencies:
  - name: subchart-a
    version: 0.1.0
    repository: file://../subchart-a
  - name: subchart-b
    version: 0.2.0
    repository: file://../subchart-b
`), utils.ReadWrite)
	require.NoError(t, err)

	chartsDir := filepath.Join(parentDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	helmClient := client.Client{Path: parentDir, Debug: true}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	// Pre-populate subchart-a as already present (fresh)
	preExisting := filepath.Join(chartsDir, "subchart-a-0.1.0.tgz")
	err = os.WriteFile(preExisting, []byte("pre-existing"), utils.ReadWrite)
	require.NoError(t, err)
	preExistingInfo, err := os.Stat(preExisting)
	require.NoError(t, err)

	// Only update subchart-b (the stale one)
	staleDep := helmClient.Chart.Metadata.Dependencies[1]
	err = helmClient.UpdateDeps([]*chart.Dependency{staleDep})
	require.NoError(t, err)

	// Verify subchart-b was downloaded
	downloadedArchive := filepath.Join(chartsDir, "subchart-b-0.2.0.tgz")
	_, err = os.Stat(downloadedArchive)
	require.NoError(t, err, "subchart-b archive should exist after selective download")

	// Verify subchart-a was NOT re-downloaded (modtime unchanged)
	afterInfo, err := os.Stat(preExisting)
	require.NoError(t, err)
	assert.Equal(t, preExistingInfo.ModTime(), afterInfo.ModTime(),
		"pre-existing dependency should not have been modified")
}

func TestUpdateDepsInvalidRepo(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), []byte(`apiVersion: v2
name: test
version: 0.1.0
`), utils.ReadWrite)
	require.NoError(t, err)

	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	helmClient := client.Client{Path: tempDir, Debug: true}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	err = helmClient.UpdateDeps([]*chart.Dependency{
		{Name: "nonexistent", Version: "1.0.0", Repository: "https://invalid.example.com/charts"},
	})
	require.Error(t, err, "should fail for invalid repository URL")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestStaleDependenciesMultipleDeps(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	chartFile := filepath.Join(tempDir, "Chart.yaml")
	err := os.WriteFile(chartFile, []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: redis
    version: 6.0.0
  - name: nginx
    version: 1.0.0
  - name: postgres
    version: 15.0.0
`), utils.ReadWrite)
	require.NoError(t, err)

	// Load the chart before creating charts/ so loader.LoadDir does not
	// attempt to unpack dummy archive files.
	helmClient := client.Client{
		Path:  tempDir,
		Debug: true,
		TTL:   time.Now().Add(-10 * time.Minute),
	}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	// Pre-populate redis and postgres as fresh, leave nginx missing
	err = os.WriteFile(filepath.Join(chartsDir, "redis-6.0.0.tgz"), []byte("dummy"), utils.ReadWrite)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(chartsDir, "postgres-15.0.0.tgz"), []byte("dummy"), utils.ReadWrite)
	require.NoError(t, err)

	staleDeps := helmClient.StaleDependencies()
	require.Len(t, staleDeps, 1, "should return only the missing dependency")
	assert.Equal(t, "nginx", staleDeps[0].Name)
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
`), utils.ReadWrite)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Create a dummy charts directory to simulate dependencies
	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
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
`), utils.ReadWrite)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Create charts directory and dependency files
	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.Mkdir(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err, "Failed to create charts directory")

	helmClient := client.Client{Path: tempDir, Debug: true}
	// Downloaded file used to contain v in the version, but downloadAndStandardize now renames it.
	archiveFile := client.GetArchiveName(chartName, chartVersion)

	err = helmClient.LoadChart()
	require.NoError(t, err, "Failed to load chart")

	err = helmClient.UpdateDeps(helmClient.Chart.Metadata.Dependencies)
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

	// Use Chart.yaml dependency versions for caching (matches standardized filenames).
	cacheDeps := helmClient.Chart.Metadata.Dependencies

	// Verify that dependency files were copied to cache
	err = helmClient.CacheDependencies(cacheDeps)
	require.NoError(t, err, "CacheDependencies should not return an error")

	cachedChart := filepath.Join(cacheDir, client.ChartsFolder, archiveFile)
	_, err = os.Stat(cachedChart)
	require.NoError(t, err, archiveFile+" should exist in cache")
	_, err = os.ReadFile(cachedChart)
	require.NoError(t, err, "Failed to read cached test file")

	// Test case: StoreDeps handles missing dependency files gracefully
	err = os.Remove(archivedChart)
	require.NoError(t, err, "Failed to remove test dependency file")

	err = helmClient.CacheDependencies(cacheDeps)
	assert.Contains(t, err.Error(), fmt.Sprintf("failed to copy chart %s to cache directory", archiveFile))

	// Test case: StoreDeps with no dependencies is a no-op
	err = helmClient.CacheDependencies(nil)
	assert.NoError(t, err, "CacheDependencies should not return an error for empty list")
}

func TestReadChartDependenciesReloadsAfterUpdate(t *testing.T) {
	tempDir := t.TempDir()

	// Create a sub-chart with its own Chart.yaml and values.yaml
	subChartDir := filepath.Join(tempDir, "subchart")
	err := os.MkdirAll(subChartDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: subchart
version: 0.1.0
`), utils.ReadWrite)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "values.yaml"), []byte(`replicaCount: 3
`), utils.ReadWrite)
	require.NoError(t, err)

	// Create the parent chart referencing the sub-chart via file://
	parentDir := filepath.Join(tempDir, "parent")
	err = os.MkdirAll(parentDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(parentDir, "Chart.yaml"), []byte(`apiVersion: v2
name: parent-chart
version: 0.1.0
dependencies:
  - name: subchart
    version: 0.1.0
    repository: file://../subchart
`), utils.ReadWrite)
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
	err := os.MkdirAll(subChartDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: subchart
version: 0.1.0
`), utils.ReadWrite)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subChartDir, "values.yaml"), []byte(`enabled: true
`), utils.ReadWrite)
	require.NoError(t, err)

	// Parent chart with the same dependency listed twice
	parentDir := filepath.Join(tempDir, "parent")
	err = os.MkdirAll(parentDir, utils.ReadWriteDir)
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
`), utils.ReadWrite)
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

func TestStaleDependenciesForceUpdate(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: redis
    version: 6.0.0
  - name: nginx
    version: 1.0.0
`), utils.ReadWrite)
	require.NoError(t, err)

	helmClient := client.Client{
		Path:               tempDir,
		Debug:              true,
		TTL:                time.Now().Add(-10 * time.Minute),
		UpdateDependencies: true,
	}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	staleDeps := helmClient.StaleDependencies()
	assert.Len(t, staleDeps, 2, "force update should return all dependencies")
}

func TestLookForArchiveCacheHit(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: mylib
    version: 2.0.0
`), utils.ReadWrite)
	require.NoError(t, err)

	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.MkdirAll(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	chartsCacheDir := filepath.Join(cacheDir, client.ChartsFolder)
	err = os.MkdirAll(chartsCacheDir, utils.ReadWriteDir)
	require.NoError(t, err)

	// Place a fresh archive in the cache (not in charts/)
	cachedArchive := filepath.Join(chartsCacheDir, "mylib-2.0.0.tgz")
	err = os.WriteFile(cachedArchive, []byte("cached-content"), utils.ReadWrite)
	require.NoError(t, err)

	helmClient := client.Client{
		Path:  tempDir,
		Debug: true,
		TTL:   time.Now().Add(-10 * time.Minute),
	}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	// StaleDependencies should find the archive in cache, copy it to charts/, and return empty
	staleDeps := helmClient.StaleDependencies()
	assert.Empty(t, staleDeps, "should not be stale when archive exists in cache")

	// Verify the archive was copied from cache to charts/
	copiedArchive := filepath.Join(chartsDir, "mylib-2.0.0.tgz")
	data, err := os.ReadFile(copiedArchive)
	require.NoError(t, err)
	assert.Equal(t, "cached-content", string(data))
}

func TestLookForArchiveCacheExpired(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
dependencies:
  - name: mylib
    version: 2.0.0
`), utils.ReadWrite)
	require.NoError(t, err)

	chartsDir := filepath.Join(tempDir, client.ChartsFolder)
	err = os.MkdirAll(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	chartsCacheDir := filepath.Join(cacheDir, client.ChartsFolder)
	err = os.MkdirAll(chartsCacheDir, utils.ReadWriteDir)
	require.NoError(t, err)

	// Place an expired archive in the cache
	cachedArchive := filepath.Join(chartsCacheDir, "mylib-2.0.0.tgz")
	err = os.WriteFile(cachedArchive, []byte("old-content"), utils.ReadWrite)
	require.NoError(t, err)

	// Set modtime to the past so it's older than the TTL cutoff
	oldTime := time.Now().Add(-1 * time.Hour)
	err = os.Chtimes(cachedArchive, oldTime, oldTime)
	require.NoError(t, err)

	helmClient := client.Client{
		Path:  tempDir,
		Debug: true,
		TTL:   time.Now().Add(-10 * time.Minute), // cutoff = 10 min ago; archive is 1 hour old
	}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	staleDeps := helmClient.StaleDependencies()
	require.Len(t, staleDeps, 1, "should be stale when cache is expired")
	assert.Equal(t, "mylib", staleDeps[0].Name)

	// Verify the expired archive was removed from cache
	_, err = os.Stat(cachedArchive)
	assert.True(t, os.IsNotExist(err), "expired cached archive should have been deleted")
}

func TestPackageLocalDependencyAbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", cacheDir)

	// Create subchart at an absolute path
	subDir := filepath.Join(tempDir, "absolute-sub")
	err := os.MkdirAll(subDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subDir, "Chart.yaml"), []byte(`apiVersion: v2
name: absolute-sub
version: 0.3.0
`), utils.ReadWrite)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subDir, "values.yaml"), []byte("key: val\n"), utils.ReadWrite)
	require.NoError(t, err)

	// Parent chart using absolute file:// path
	parentDir := filepath.Join(tempDir, "parent")
	err = os.MkdirAll(parentDir, utils.ReadWriteDir)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(parentDir, "Chart.yaml"), fmt.Appendf(nil, `apiVersion: v2
name: parent
version: 0.1.0
dependencies:
  - name: absolute-sub
    version: 0.3.0
    repository: file://%s
`, subDir), utils.ReadWrite)
	require.NoError(t, err)

	chartsDir := filepath.Join(parentDir, client.ChartsFolder)
	err = os.MkdirAll(chartsDir, utils.ReadWriteDir)
	require.NoError(t, err)

	helmClient := client.Client{Path: parentDir, Debug: true}
	err = helmClient.LoadChart()
	require.NoError(t, err)

	err = helmClient.UpdateDeps(helmClient.Chart.Metadata.Dependencies)
	require.NoError(t, err)

	archive := filepath.Join(chartsDir, "absolute-sub-0.3.0.tgz")
	_, err = os.Stat(archive)
	require.NoError(t, err, "archive should exist after packaging with absolute path")
}

func TestStandardizeArchivePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileName     string
		depName      string
		depVersion   string
		expectedFile string
	}{
		{
			name:         "helm suffix is removed",
			fileName:     "flink-kubernetes-operator-1.14.0-helm.tgz",
			depName:      "flink-kubernetes-operator",
			depVersion:   "1.14.0",
			expectedFile: "flink-kubernetes-operator-1.14.0.tgz",
		},
		{
			name:         "v prefix in version is removed",
			fileName:     "cert-manager-v1.18.2.tgz",
			depName:      "cert-manager",
			depVersion:   "1.18.2",
			expectedFile: "cert-manager-1.18.2.tgz",
		},
		{
			name:         "v prefix and helm suffix combined",
			fileName:     "flink-kubernetes-operator-v1.14.0-helm.tgz",
			depName:      "flink-kubernetes-operator",
			depVersion:   "1.14.0",
			expectedFile: "flink-kubernetes-operator-1.14.0.tgz",
		},
		{
			name:         "already canonical name is not modified",
			fileName:     "redis-6.0.0.tgz",
			depName:      "redis",
			depVersion:   "6.0.0",
			expectedFile: "redis-6.0.0.tgz",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			srcPath := filepath.Join(dir, test.fileName)
			err := os.WriteFile(srcPath, []byte("archive-content"), utils.ReadWrite)
			require.NoError(t, err)

			err = client.StandardizeArchivePath(srcPath, test.depName, test.depVersion)
			require.NoError(t, err)

			expectedPath := filepath.Join(dir, test.expectedFile)
			data, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "expected file %s to exist", test.expectedFile)
			assert.Equal(t, "archive-content", string(data))

			if test.fileName != test.expectedFile {
				_, err = os.Stat(srcPath)
				assert.True(t, os.IsNotExist(err), "original file %s should have been renamed", test.fileName)
			}
		})
	}
}
