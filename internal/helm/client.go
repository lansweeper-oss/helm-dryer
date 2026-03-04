// Package client provides functions to operate with the Helm SDK client.
package client

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lansweeper/helm-dryer/internal/cli"
	"github.com/lansweeper/helm-dryer/internal/utils"
	"github.com/lansweeper/helm-dryer/internal/values"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	helmCli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	ociRegistry "helm.sh/helm/v3/pkg/registry"
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
	PassPending     bool
	TemplateOptions string
}

// dependencyKey returns a unique key for deduplication.
func dependencyKey(dep *chart.Dependency) string {
	return dep.Name + "|" + dep.Version + "|" + dep.Repository
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

// DependenciesNeedUpdate checks if the dependencies of a Helm chart need to be updated.
func (h *Client) DependenciesNeedUpdate() bool {
	for _, dependency := range h.Chart.Metadata.Dependencies {
		slog.Debug("Checking dependency: " + dependency.Name + " version: " + dependency.Version)

		exists := h.lookForArchive(dependency.Name, dependency.Version)
		if !exists {
			slog.Debug("Dependency not found, triggering an update")

			return true
		}
	}

	return false
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

	if h.UpdateDependencies || h.DependenciesNeedUpdate() {
		err = h.UpdateDeps()
		if err != nil {
			return nil, fmt.Errorf("could not update dependencies: %w", err)
		}

		err = h.CacheDependencies()
		if err != nil {
			return nil, fmt.Errorf("could not store chart dependencies: %w", err)
		}

		// Reload the chart so that Dependencies() reflects the newly downloaded archives.
		h.Chart, err = loader.LoadDir(h.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to reload chart after dependency update: %w", err)
		}
	}

	return h.ReadDependenciesValues()
}

// ReadDependenciesValues returns a map of values from the dependencies of a Helm chart.
// It reads the values files from the dependencies and merges them into a single map where the
// root keys are the names of the dependencies.
func (h *Client) ReadDependenciesValues() (map[string]any, error) {
	dir := filepath.Join(h.Path, "charts")
	vals := map[string]any{}

	for _, dependency := range h.Chart.Dependencies() {
		slog.Debug("Reading values for dependency", "name", dependency.Metadata.Name)
		archive := fmt.Sprintf("%s-%s.tgz", dependency.Metadata.Name, dependency.Metadata.Version)
		fullPath := filepath.Join(dir, archive)

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
func (h *Client) UpdateDeps() error {
	cacheDir := getCacheDir()
	slog.Debug(fmt.Sprintf("Updating dependencies from: %s (cache: %s)", h.Path, cacheDir))
	settings := helmCli.EnvSettings{
		Debug:           h.Debug,
		RepositoryCache: cacheDir,
	}
	// We need to override this since Helm internals do not honor the getter settings :(
	err := os.Setenv(Cache, cacheDir)
	if err != nil {
		return fmt.Errorf("failed to set environment variable %s: %w", Cache, err)
	}

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
		return fmt.Errorf("failed to create OCI registry client: %w", err)
	}

	if opt != nil {
		err = registryClient.Login(h.Credentials.Registry, opt)
		if err != nil {
			return fmt.Errorf("failed to login to OCI registry: %w", err)
		}
	}

	manager := &downloader.Manager{
		ChartPath:       h.Path,
		Getters:         getter.All(&settings),
		Out:             os.Stderr,
		RegistryClient:  registryClient,
		RepositoryCache: cacheDir,
		SkipUpdate:      false,
		Verify:          downloader.VerifyNever,
	}

	err = manager.Update()
	if err != nil {
		return fmt.Errorf("failed to update dependencies: %w", err)
	}

	slog.Debug("Dependencies updated")

	return nil
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
