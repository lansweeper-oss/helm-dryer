package client

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"helm.sh/helm/v3/pkg/chart"
)

const (
	Cache = "HELM_CACHE_HOME"
)

// EnsureCacheDirs ensures that the Helm cache directories exists.
// It also sets HELM_CACHE_HOME if unset, so the Helm SDK (e.g. helmpath.CachePath)
// resolves repository index paths to the same writable directory.
func EnsureCacheDirs(path string) error {
	chartDependenciesDir := filepath.Join(path, ChartsFolder)
	cacheDir := getCacheDir()
	chartsCacheDir := getChartsCacheDir()

	if os.Getenv(Cache) == "" {
		err := os.Setenv(Cache, cacheDir)
		if err != nil {
			slog.Warn(
				"Helm cache directory falls back to /.cache, ensure this is writable or set HELM_CACHE_HOME",
				"cacheDir", cacheDir, "err", err,
			)
		}
	}

	err := os.MkdirAll(chartDependenciesDir, utils.ReadWriteDir)
	if err != nil {
		return fmt.Errorf("failed to ensure chart dependencies directory %s exists: %w", chartDependenciesDir, err)
	}

	err = os.MkdirAll(cacheDir, utils.ReadWriteDir)
	if err != nil {
		return fmt.Errorf("failed to ensure cache directory %s exists: %w", cacheDir, err)
	}

	err = os.MkdirAll(chartsCacheDir, utils.ReadWriteDir)
	if err != nil {
		return fmt.Errorf("failed to ensure charts cache directory %s exists: %w", chartsCacheDir, err)
	}

	return nil
}

// GetArchiveName returns the name of the archive for a given chart name and version.
func GetArchiveName(name, version string) string {
	return fmt.Sprintf("%s-%s.tgz", name, version)
}

// getCacheDir returns the directory where Helm caches its charts.
func getCacheDir() string {
	return utils.GetEnv(Cache, filepath.Join(os.TempDir(), "helm-cache"))
}

func getChartsCacheDir() string {
	return filepath.Join(getCacheDir(), ChartsFolder)
}

// CacheDependencies copies the chart tgz files to the cache directory.
func (h *Client) CacheDependencies(dependencies []*chart.Dependency) error {
	dir := filepath.Join(h.Path, ChartsFolder)
	cacheDir := getChartsCacheDir()

	for _, dependency := range dependencies {
		archivedChart := GetArchiveName(dependency.Name, dependency.Version)

		slog.Debug("Storing chart " + archivedChart)
		sourcePath := filepath.Join(dir, archivedChart)
		cachePath := filepath.Join(cacheDir, archivedChart)

		err := utils.CopyFile(sourcePath, cachePath, cacheDir)
		if err != nil {
			return fmt.Errorf("failed to copy chart %s to cache directory: %w", archivedChart, err)
		}
	}

	return nil
}

// lookForArchive checks if a chart archive exists in the specified path or in the cache.
// It returns false when either the archive is not present, the TTL is expired, or no TTL is set.
func (h *Client) lookForArchive(name string, version string) bool {
	// If no TTL is configured, caching is disabled, always require a fresh download.
	if h.TTL.IsZero() {
		return false
	}

	dir := filepath.Join(h.Path, ChartsFolder)
	chartsCacheDir := getChartsCacheDir()
	archive := GetArchiveName(name, version)
	dependencyArchive := filepath.Join(dir, archive)
	cachedDependency := filepath.Join(chartsCacheDir, archive)

	dependencyStatInfo, err := os.Stat(dependencyArchive)

	// If the archive exists and is newer than the TTL, we can use it directly
	if err == nil && dependencyStatInfo.ModTime().After(h.TTL) {
		return true
	}

	cachedStatInfo, err := os.Stat(cachedDependency)

	switch {
	case err != nil:
		return false
	case cachedStatInfo.ModTime().Before(h.TTL):
		slog.Debug("Chart " + archive + " found in cache, but TTL is expired")

		err = os.Remove(cachedDependency)
		if err != nil {
			slog.Warn("Failed to remove expired cached chart", "path", cachedDependency, "err", err)
		}

		return false
	default:
		err = utils.CopyFile(cachedDependency, dependencyArchive, dir)
		if err != nil {
			slog.Warn("failed to copy chart from cache", "archive", archive, "dir", dir, "err", err)

			return false
		}

		slog.Debug("Chart " + archive + " copied from cache")
	}

	return true
}
