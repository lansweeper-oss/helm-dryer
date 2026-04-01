package client

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/lansweeper-oss/helm-dryer/internal/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	helmCli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	ociRegistry "helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
)

func (h *Client) FindBestVersionMatch(availableVersions []string, constraint string) (string, error) {
	// Use semver package to find best match
	// Or iterate and find the highest version that satisfies constraint
	versionConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", errors.ErrConstraintNotSatisfied
	}

	var latestVersion *semver.Version

	var latestTag string

	for _, tag := range availableVersions {
		version, err := semver.NewVersion(tag)
		if err != nil {
			continue // Skip non-semver tags
		}

		if versionConstraint.Check(version) {
			if latestVersion == nil || version.GreaterThan(latestVersion) {
				latestVersion = version
				latestTag = tag
			}
		}
	}

	if latestTag != "" {
		return latestTag, nil
	}

	return "", errors.ErrConstraintNotSatisfied
}

// ResolveVersion resolves the version of a chart dependency.
func (h *Client) ResolveVersion(dependency *chart.Dependency) (string, error) {
	// Return the version directly if it's already a specific version (not a constraint)
	_, err := semver.NewVersion(dependency.Version)
	if err == nil {
		return dependency.Version, nil
	}

	if strings.HasPrefix(dependency.Repository, LocalRepoPrefix) {
		return h.resolveLocalVersion(dependency)
	}

	if ociRegistry.IsOCI(dependency.Repository) {
		return h.resolveOCIVersion(dependency)
	}

	return h.resolveHTTPVersion(dependency)
}

func (h *Client) resolveHTTPVersion(dep *chart.Dependency) (string, error) {
	settings := helmCli.EnvSettings{
		Debug:           h.Debug,
		RepositoryCache: getCacheDir(),
	}
	// Load the repository index
	chartRepo, err := repo.NewChartRepository(
		&repo.Entry{
			Name: dep.Name,
			URL:  dep.Repository,
		},
		getter.All(&settings),
	)
	if err != nil {
		return "", fmt.Errorf("failed to prepare repository before loading its index: %w", err)
	}

	// Download the index file (lightweight, just metadata)
	indexFile, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return "", fmt.Errorf("failed to download index: %w", err)
	}

	index, err := repo.LoadIndexFile(indexFile)
	if err != nil {
		return "", fmt.Errorf("failed to load index: %w", err)
	}

	version, err := index.Get(dep.Name, dep.Version)
	if err != nil {
		return "", fmt.Errorf("failed to find version %s for chart %s in index: %w", dep.Version, dep.Name, err)
	}

	return version.Version, nil
}

// resolveLocalVersion resolves a local chart dependency by validating its version
// against the constraint.
func (h *Client) resolveLocalVersion(dep *chart.Dependency) (string, error) {
	localPath := strings.TrimPrefix(dep.Repository, LocalRepoPrefix)
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(h.Path, localPath)
	}

	localPath = filepath.Clean(localPath)

	// Load only Chart.yaml metadata (lightweight) instead of the entire chart
	chartFilePath := filepath.Join(localPath, "Chart.yaml")
	metadata, err := chartutil.LoadChartfile(chartFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to load chart metadata from %s: %w", chartFilePath, err)
	}

	localVersion := metadata.Version

	// Validate that the local chart's version satisfies the constraint
	versionConstraint, err := semver.NewConstraint(dep.Version)
	if err != nil {
		return "", fmt.Errorf("invalid version constraint %s: %w", dep.Version, err)
	}

	version, err := semver.NewVersion(localVersion)
	if err != nil {
		return "", fmt.Errorf("invalid version in local chart %s: %w", localVersion, err)
	}

	if !versionConstraint.Check(version) {
		return "", errors.ErrConstraintNotSatisfied
	}

	return localVersion, nil
}

// resolveOCIVersion resolves the version of an OCI chart dependency by listing available tags
// and finding the best match.
func (h *Client) resolveOCIVersion(dep *chart.Dependency) (string, error) {
	registryClient, err := h.registryClient()
	if err != nil {
		return "", fmt.Errorf("failed to create registry client: %w", err)
	}

	ref := strings.TrimPrefix(dep.Repository, "oci://") + "/" + dep.Name

	tags, err := registryClient.Tags(ref)
	if err != nil {
		return "", fmt.Errorf("failed to list OCI tags for %s: %w", ref, err)
	}

	if len(tags) == 0 {
		return "", errors.ErrNotFound
	}

	// Find the best matching version using semver constraints
	// OCI tags are typically semantic versions
	bestMatch, err := h.FindBestVersionMatch(tags, dep.Version)
	if err != nil {
		return "", errors.ErrNotFound
	}

	return bestMatch, nil
}
