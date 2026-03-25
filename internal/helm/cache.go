package client

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

var errArchiveNotFound = errors.New("failed to find archive")

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

// GetCanonicalArchiveName returns the canonical archive filename for a given chart name and version.
func GetCanonicalArchiveName(name, version string) string {
	return fmt.Sprintf("%s-%s.tgz", name, version)
}

// resolveArchive tries the conventional name-version.tgz first. If that file doesn't exist,
// it falls back to scanning dir for any .tgz whose embedded Chart metadata matches name and version.
// Returns the full path to the archive, or "" if not found.
func resolveArchive(dir, name, version string) string {
	conventional := filepath.Join(dir, GetCanonicalArchiveName(name, version))

	_, err := os.Stat(conventional)
	if err == nil {
		return conventional
	}

	return findArchiveByMetadata(dir, name, version)
}

// findArchiveByMetadata scans dir for .tgz files and returns the full path of the first one
// whose Chart.yaml metadata matches the given name and version.
func findArchiveByMetadata(dir, name, version string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.tgz"))
	if err != nil || len(matches) == 0 {
		return ""
	}

	for _, match := range matches {
		archiveFile, err := os.Open(filepath.Clean(match))
		if err != nil {
			continue
		}

		loadedChart, err := loader.LoadArchive(archiveFile)

		_ = archiveFile.Close()

		if err != nil {
			continue
		}

		if loadedChart.Metadata.Name == name && loadedChart.Metadata.Version == version {
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
		sourcePath := resolveArchive(dir, dependency.Name, dependency.Version)
		if sourcePath == "" {
			return fmt.Errorf("%w for %s-%s in %s", errArchiveNotFound, dependency.Name, dependency.Version, dir)
		}

		archiveName := GetCanonicalArchiveName(dependency.Name, dependency.Version)
		canonicalPath := filepath.Join(dir, archiveName)

		// Rename to canonical name in charts/ so future lookups hit the fast os.Stat path.
		if sourcePath != canonicalPath {
			if err := os.Rename(sourcePath, canonicalPath); err != nil {
				return fmt.Errorf("failed to rename %s to %s: %w", filepath.Base(sourcePath), archiveName, err)
			}
		}

		cachePath := filepath.Join(cacheDir, archiveName)

		slog.Debug("Storing chart " + archiveName)

		err := utils.CopyFile(canonicalPath, cachePath)
		if err != nil {
			return fmt.Errorf("failed to copy chart %s to cache directory: %w", archiveName, err)
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

	// Check charts/ directory
	dependencyArchive := resolveArchive(dir, name, version)
	if dependencyArchive != "" {
		dependencyStatInfo, err := os.Stat(dependencyArchive)
		if err == nil && dependencyStatInfo.ModTime().After(h.TTL) {
			return true
		}
	}

	// Check cache directory
	cachedDependency := resolveArchive(chartsCacheDir, name, version)
	if cachedDependency == "" {
		return false
	}

	cachedStatInfo, err := os.Stat(cachedDependency)
	if err != nil {
		return false
	}

	archiveName := GetCanonicalArchiveName(name, version)

	if cachedStatInfo.ModTime().Before(h.TTL) {
		slog.Debug("Chart " + archiveName + " found in cache, but TTL is expired")

		err = os.Remove(cachedDependency)
		if err != nil {
			slog.Warn("Failed to remove expired cached chart", "path", cachedDependency, "err", err)
		}

		return false
	}

	destPath := filepath.Join(dir, archiveName)

	err = utils.CopyFile(cachedDependency, destPath)
	if err != nil {
		slog.Warn("failed to copy chart from cache", "archive", archiveName, "dir", dir, "err", err)

		return false
	}

	slog.Debug("Chart " + archiveName + " copied from cache")

	return true
}
