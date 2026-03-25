package client

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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

// GetConventionalArchiveName returns the conventional archive filename for a given chart name and version.
func GetConventionalArchiveName(name, version string) string {
	return fmt.Sprintf("%s-%s.tgz", name, version)
}

// resolveArchive tries the conventional name-version.tgz first. If that file doesn't exist,
// it falls back to scanning dir for any .tgz whose filename contains both the exact name and version.
// Returns the "actual" file name if found, or an empty string if no matching archive is found.
func resolveArchiveName(dir, name, version string) string {
	conventional := filepath.Join(dir, GetConventionalArchiveName(name, version))

	_, err := os.Stat(conventional)
	if err == nil {
		return conventional
	}

	return findArchiveByName(dir, name, version)
}

// findArchiveByName scans dir for .tgz files and returns the full path of the first one
// whose filename contains both the exact name and version.
func findArchiveByName(dir, name, version string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.tgz"))
	if err != nil || len(matches) == 0 {
		return ""
	}

	for _, match := range matches {
		base := filepath.Base(match)
		if strings.Contains(base, name) && strings.Contains(base, version) {
			return match
		}
	}

	return ""
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
		sourcePath := resolveArchiveName(dir, dependency.Name, dependency.Version)
		if sourcePath == "" {
			return fmt.Errorf("failed to find chart archive for %s-%s in %s", dependency.Name, dependency.Version, dir)
		}

		cachedChart := GetConventionalArchiveName(dependency.Name, dependency.Version)
		slog.Debug("Storing chart " + cachedChart)
		cachePath := filepath.Join(cacheDir, cachedChart)

		err := utils.CopyFile(sourcePath, cachePath)
		if err != nil {
			return fmt.Errorf("failed to copy chart %s to cache directory: %w", cachedChart, err)
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
	conventionalArchive := GetConventionalArchiveName(name, version)
	dependencyArchive := resolveArchiveName(dir, name, version)
	cachedDependency := filepath.Join(chartsCacheDir, conventionalArchive)

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
		slog.Debug("Chart " + conventionalArchive + " found in cache, but TTL is expired")

		err = os.Remove(cachedDependency)
		if err != nil {
			slog.Warn("Failed to remove expired cached chart", "path", cachedDependency, "err", err)
		}

		return false
	default:
		// Copy from cache to charts/ using the conventional name
		target := filepath.Join(dir, conventionalArchive)
		err = utils.CopyFile(cachedDependency, target)
		if err != nil {
			slog.Warn("failed to copy chart from cache", "archive", conventionalArchive, "dir", dir, "err", err)

			return false
		}

		slog.Debug("Chart " + conventionalArchive + " copied from cache")
	}

	return true
}
