// Package client provides functions to operate with the Helm SDK client.
package client

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lansweeper-oss/helm-dryer/internal/cli"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/lansweeper-oss/helm-dryer/internal/values"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	helmCli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	ociRegistry "helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	ChartsFolder    = "charts"
	LocalRepoPrefix = "file://"
)

// Client is a client for interacting with Helm charts.
type Client struct {
	Chart              *chart.Chart
	Credentials        *cli.Credentials
	Debug              bool
	Path               string
	TTL                time.Time
	UpdateDependencies bool
}

type Options struct {
	DelimLeft       string
	DelimRight      string
	TemplateOptions string
}

// TemplateAndParseYaml reads a YAML file, applies a template to it, and returns the resulting data as a map.
func TemplateAndParseYaml(
	file string, options Options,
	vals, runtime map[string]any,
) (map[string]any, error) {
	// #nosec G304
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", file, err)
	}

	tpl := utils.GetTemplate(options.TemplateOptions, options.DelimLeft, options.DelimRight)

	_, err = tpl.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template for file %s: %w", file, err)
	}

	// Add Values key to the runtime map, so we end up with what Helm expects.
	// NOTE: this mutates the caller's runtime map. This is intentional: processValuesFiles passes
	// the same runtimeValues reference for every file, and the merge is idempotent (same key/value
	// each time). Creating a copy per call would be safer but adds allocation overhead for no
	// observable benefit given the current call sites.
	err = values.MergeYamlMaps(
		runtime,
		map[string]any{
			"Values": vals,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to merge values for file %s: %w", file, err)
	}

	// This part is a re-implementation of the low-level Helm engine.render private method
	var templatedContent bytes.Buffer

	err = tpl.Execute(&templatedContent, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template for file %s: %w", file, err)
	}

	yamlData, err := utils.ParseYAML(templatedContent.Bytes())
	if err != nil {
		slog.Debug(templatedContent.String())

		return nil, fmt.Errorf("failed to unmarshal YAML for file %s: %w", file, err)
	}

	return yamlData, nil
}

// StaleDependencies returns which Helm chart dependencies need an update.
func (h *Client) StaleDependencies() []*chart.Dependency {
	if h.UpdateDependencies {
		return h.Chart.Metadata.Dependencies
	}

	needUpdate := []*chart.Dependency{}

	for _, dependency := range h.Chart.Metadata.Dependencies {
		slog.Debug("Checking dependency: " + dependency.Name + " version: " + dependency.Version)

		exists := h.lookForArchive(dependency.Name, dependency.Version)
		if !exists {
			slog.Debug("Dependency not found, triggering an update")

			needUpdate = append(needUpdate, dependency)
		}
	}

	return needUpdate
}

// HasDependencies checks if the Helm chart has any dependencies defined in its Chart.yaml file.
func (h *Client) HasDependencies() bool {
	if h.Chart == nil || h.Chart.Metadata == nil {
		return false
	}

	return len(h.Chart.Metadata.Dependencies) > 0
}

// LoadChart loads a Helm chart.
func (h *Client) LoadChart() error {
	chart, err := loader.LoadDir(h.Path)
	if err != nil {
		return fmt.Errorf("failed to load chart from path %s: %w", h.Path, err)
	}

	h.Chart = chart

	return nil
}

// ReadChartDependencies loads the Chart (by ignoring any templated values file), goes through its
// dependencies, update if needed and obtains the values from those.
func (h *Client) ReadChartDependencies() (map[string]any, error) {
	err := EnsureCacheDirs(h.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure cache directory: %w", err)
	}

	h.Chart, err = loader.LoadDir(h.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to load Chart.yaml: %w", err)
	}

	if len(h.Chart.Metadata.Dependencies) == 0 {
		slog.Debug("No dependencies found", "chart", h.Chart.Metadata.Name)

		return map[string]any{}, nil
	}

	h.deduplicateDependencies()

	dependenciesToUpdate := h.StaleDependencies()

	if len(dependenciesToUpdate) > 0 {
		err = h.UpdateDeps(dependenciesToUpdate)
		if err != nil {
			return nil, fmt.Errorf("could not update dependencies: %w", err)
		}

		// Reload the chart so that Dependencies() reflects the newly downloaded archives
		// with their actual resolved versions (not the constraint strings from Chart.yaml).
		h.Chart, err = loader.LoadDir(h.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to reload chart after dependency update: %w", err)
		}

		// Build cache deps from loaded sub-charts to get resolved versions (e.g. "0.0.9"
		// instead of the constraint "~0.0.9" that was in Chart.Metadata.Dependencies).
		cacheDeps := make([]*chart.Dependency, 0, len(h.Chart.Dependencies()))
		for _, c := range h.Chart.Dependencies() {
			cacheDeps = append(cacheDeps, &chart.Dependency{
				Name:    c.Metadata.Name,
				Version: c.Metadata.Version,
			})
		}

		err = h.CacheDependencies(cacheDeps)
		if err != nil {
			return nil, fmt.Errorf("could not store chart dependencies: %w", err)
		}
	}

	return h.ReadDependenciesValues()
}

// ReadDependenciesValues returns a map of values from the dependencies of a Helm chart.
// It reads the values files from the dependencies and merges them into a single map where the
// root keys are the names of the dependencies.
func (h *Client) ReadDependenciesValues() (map[string]any, error) {
	dir := filepath.Join(h.Path, ChartsFolder)
	vals := map[string]any{}

	for _, dependency := range h.Chart.Dependencies() {
		slog.Debug("Reading values for dependency", "name", dependency.Metadata.Name)

		fullPath := resolveArchiveName(dir, dependency.Metadata.Name, dependency.Metadata.Version)
		if fullPath == "" {
			return nil, fmt.Errorf("failed to find dependency chart archive for %s-%s in %s: %w",
				dependency.Metadata.Name, dependency.Metadata.Version, dir, ErrChartArchiveNotFound)
		}

		file, err := os.Open(filepath.Clean(fullPath))
		if err != nil {
			return nil, fmt.Errorf("failed to open dependency chart %s: %w", fullPath, err)
		}

		depChart, err := loader.LoadArchive(file)

		closeErr := file.Close()
		if closeErr != nil && err == nil {
			slog.Warn("failed to close dependency chart", "path", fullPath, "err", closeErr)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to load dependency chart %s: %w", fullPath, err)
		}

		if depValues := depChart.Values; len(depValues) > 0 {
			if _, exists := vals[dependency.Metadata.Name]; !exists {
				vals[dependency.Metadata.Name] = depValues
			}
		}
	}

	return vals, nil
}

// UpdateDeps updates the dependencies of a Helm chart located at the specified path.
func (h *Client) UpdateDeps(dependencies []*chart.Dependency) error {
	chartsDir := filepath.Join(h.Path, ChartsFolder)

	downloader, err := h.chartDownloader()
	if err != nil {
		return fmt.Errorf("failed to create chart downloader: %w", err)
	}

	settings := h.envSettings()

	for _, dep := range dependencies {
		switch {
		case strings.HasPrefix(dep.Repository, LocalRepoPrefix):
			err = h.packageLocalDependency(dep, chartsDir)
		case ociRegistry.IsOCI(dep.Repository):
			ref := strings.TrimRight(dep.Repository, "/") + "/" + dep.Name
			slog.Debug("Downloading OCI dependency", "ref", ref, "version", dep.Version)

			_, _, err = downloader.DownloadTo(ref, dep.Version, chartsDir)
		default:
			var chartURL string

			chartURL, err = repo.FindChartInRepoURL(
				dep.Repository, dep.Name, dep.Version,
				"", "", "",
				getter.All(&settings),
			)
			if err != nil {
				err = fmt.Errorf("failed to resolve chart URL for %s: %w", dep.Name, err)

				break
			}

			slog.Debug("Downloading HTTP dependency", "url", chartURL, "version", dep.Version)

			_, _, err = downloader.DownloadTo(chartURL, dep.Version, chartsDir)
		}

		if err != nil {
			return fmt.Errorf("failed to download dependency %s-%s: %w", dep.Name, dep.Version, err)
		}

		slog.Debug("Dependency downloaded", "name", dep.Name, "version", dep.Version)
	}

	return nil
}

// chartDownloader sets up the proper credentials and cache settings.
func (h *Client) chartDownloader() (*downloader.ChartDownloader, error) {
	settings := h.envSettings()

	registryClient, err := h.registryClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	return &downloader.ChartDownloader{
		Out:             os.Stderr,
		Verify:          downloader.VerifyNever,
		RepositoryCache: settings.RepositoryCache,
		RegistryClient:  registryClient,
		Getters:         getter.All(&settings),
	}, nil
}

// dependencyKey returns a unique key for deduplication.
func dependencyKey(dep *chart.Dependency) string {
	return dep.Name + "|" + dep.Version + "|" + dep.Repository
}

// deduplicateDependencies remove duplicates from the dependencies list.
func (h *Client) deduplicateDependencies() {
	slog.Debug("Deduplicating chart dependencies")

	seen := make(map[string]struct{}, len(h.Chart.Metadata.Dependencies))
	dependencies := make([]*chart.Dependency, 0, len(h.Chart.Metadata.Dependencies))

	for _, dependency := range h.Chart.Metadata.Dependencies {
		// We cannot rely on dependency.Enabled since apparently Helm does not honor
		// omitempty when reading the Chart.yaml file.
		key := dependencyKey(dependency)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}

			dependencies = append(dependencies, dependency)
		}
	}

	h.Chart.Metadata.Dependencies = dependencies
}

// envSettings returns a configured Helm EnvSettings instance.
func (h *Client) envSettings() helmCli.EnvSettings {
	return helmCli.EnvSettings{
		Debug:           h.Debug,
		RepositoryCache: getCacheDir(),
	}
}

// packageLocalDependency packages a local dependency into the charts directory.
func (h *Client) packageLocalDependency(dep *chart.Dependency, destDir string) error {
	localPath := strings.TrimPrefix(dep.Repository, LocalRepoPrefix)
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(h.Path, localPath)
	}

	localPath = filepath.Clean(localPath)

	slog.Debug("Packaging local dependency", "path", localPath)

	localChart, err := loader.LoadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to load local chart from %s: %w", localPath, err)
	}

	_, err = chartutil.Save(localChart, destDir)
	if err != nil {
		return fmt.Errorf("failed to package local chart %s: %w", dep.Name, err)
	}

	return nil
}

// registryClient creates and authenticates an OCI registry client using the configured credentials.
func (h *Client) registryClient() (*ociRegistry.Client, error) {
	clientOpts := []ociRegistry.ClientOption{
		ociRegistry.ClientOptDebug(h.Debug),
		ociRegistry.ClientOptEnableCache(true),
	}

	var opt ociRegistry.LoginOption

	if h.Credentials != nil {
		if h.Credentials.Username != "" && h.Credentials.Password != "" && h.Credentials.Registry != "" {
			slog.Debug("Using basic auth for OCI registry in " + h.Credentials.Registry)

			opt = ociRegistry.LoginOptBasicAuth(h.Credentials.Username, h.Credentials.Password)
		} else if h.Credentials.File != "" {
			slog.Debug("Using credentials file for OCI registry")

			clientOpts = append(clientOpts, ociRegistry.ClientOptCredentialsFile(h.Credentials.File))
		}
	}

	registryClient, err := ociRegistry.NewClient(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI registry client: %w", err)
	}

	if opt != nil {
		err = registryClient.Login(h.Credentials.Registry, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to login to OCI registry: %w", err)
		}
	}

	return registryClient, nil
}
