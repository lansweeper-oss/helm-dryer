// Package client provides functions to operate with the Helm SDK client.
package client

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lansweeper-oss/helm-dryer/internal/repocreds"
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
	CredentialsFile    string
	CredsStore         *repocreds.Store
	Debug              bool
	Path               string
	TTL                time.Time
	UpdateDependencies bool

	cachedRegistryClient *ociRegistry.Client
}

type Options struct {
	DelimLeft       string
	DelimRight      string
	TemplateOptions string
}

type resolvedCred struct {
	Username string
	Password string
	certFile string
	keyFile  string
	cleanups []string
}

// StandardizeArchivePath renames a downloaded chart archive to the canonical
// <name>-<version>.tgz format if the filename doesn't already match.
func StandardizeArchivePath(downloadedPath, name, version string) error {
	expectedPath := filepath.Join(filepath.Dir(downloadedPath), GetArchiveName(name, version))
	if downloadedPath == expectedPath {
		return nil
	}

	slog.Debug("Standardizing chart archive name",
		"from", filepath.Base(downloadedPath),
		"to", filepath.Base(expectedPath))

	err := os.Rename(downloadedPath, expectedPath)
	if err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", downloadedPath, expectedPath, err)
	}

	return nil
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

func dependencyKey(dep *chart.Dependency) string {
	return dep.Name + "|" + dep.Version + "|" + dep.Repository
}

func downloadAndStandardize(
	ctx context.Context, downloader *downloader.ChartDownloader, ref string, dep *chart.Dependency, destDir string,
) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("download cancelled: %w", ctx.Err())
	default:
	}

	downloadedPath, _, err := downloader.DownloadTo(ref, dep.Version, destDir)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("download cancelled: %w", ctx.Err())
		}

		return fmt.Errorf("failed to download %s: %w", ref, err)
	}

	return StandardizeArchivePath(downloadedPath, dep.Name, dep.Version)
}

func extractOCIHost(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, repocreds.OCISchemePrefix)

	host, _, _ := strings.Cut(trimmed, "/")

	return host
}

func ociLoginOpts(cred *resolvedCred) []ociRegistry.LoginOption {
	var opts []ociRegistry.LoginOption

	if cred.Username != "" {
		opts = append(opts, ociRegistry.LoginOptBasicAuth(cred.Username, cred.Password))
	}

	if cred.certFile != "" && cred.keyFile != "" {
		opts = append(opts, ociRegistry.LoginOptTLSClientConfig(cred.certFile, cred.keyFile, ""))
	}

	return opts
}

func writeTempPEM(data []byte) string {
	tempFile, err := os.CreateTemp("", "helm-dryer-*.pem")
	if err != nil {
		slog.Warn("Failed to create temp PEM file", "err", err)

		return ""
	}

	defer func() { _ = tempFile.Close() }()

	_, err = tempFile.Write(data)
	if err != nil {
		slog.Warn("Failed to write temp PEM file", "err", err)

		return ""
	}

	return tempFile.Name()
}

// cleanup removes any temporary PEM files created for TLS credentials.
func (rc *resolvedCred) cleanup() {
	for _, path := range rc.cleanups {
		_ = os.Remove(path)
	}
}

func (h *Client) HasDependencies() bool {
	if h.Chart == nil || h.Chart.Metadata == nil {
		return false
	}

	return len(h.Chart.Metadata.Dependencies) > 0
}

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
func (h *Client) ReadChartDependencies(ctx context.Context) (map[string]any, error) {
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

	dependenciesToUpdate := h.StaleDependencies(ctx)

	if len(dependenciesToUpdate) > 0 {
		err = h.UpdateDeps(ctx, dependenciesToUpdate)
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
	vals := map[string]any{}

	for _, dependency := range h.Chart.Dependencies() {
		slog.Debug("Reading values for dependency", "name", dependency.Metadata.Name)

		if depValues := dependency.Values; len(depValues) > 0 {
			if _, exists := vals[dependency.Metadata.Name]; !exists {
				vals[dependency.Metadata.Name] = depValues
			}
		}
	}

	return vals, nil
}

// StaleDependencies returns which Helm chart dependencies need an update.
func (h *Client) StaleDependencies(ctx context.Context) []*chart.Dependency {
	if h.UpdateDependencies {
		return h.Chart.Metadata.Dependencies
	}

	needUpdate := []*chart.Dependency{}

	for _, dependency := range h.Chart.Metadata.Dependencies {
		version, err := h.ResolveVersion(ctx, dependency)
		if err != nil {
			slog.Warn(
				"Failed to resolve version for dependency, will attempt update",
				"name", dependency.Name,
				"version", dependency.Version,
				"repository", dependency.Repository,
				"err", err,
			)

			needUpdate = append(needUpdate, dependency)

			continue
		}
		// Update the dependency version to the resolved one and work with that from now on.
		dependency.Version = version
		slog.Debug("Checking dependency: " + dependency.Name + " version: " + version)

		exists := h.lookForArchive(dependency.Name, version)
		if !exists {
			slog.Debug("Dependency not found, triggering an update")

			needUpdate = append(needUpdate, dependency)
		}
	}

	return needUpdate
}

// UpdateDeps updates the dependencies of a Helm chart located at the specified path.
// It respects the provided context for cancellation and timeout.
func (h *Client) UpdateDeps(ctx context.Context, dependencies []*chart.Dependency) error {
	chartsDir := filepath.Join(h.Path, ChartsFolder)

	downloader, err := h.chartDownloader(nil)
	if err != nil {
		return fmt.Errorf("failed to create chart downloader: %w", err)
	}

	settings := h.envSettings()

	for _, dep := range dependencies {
		select {
		case <-ctx.Done():
			return fmt.Errorf("dependency download cancelled: %w", ctx.Err())
		default:
		}

		switch {
		case strings.HasPrefix(dep.Repository, LocalRepoPrefix):
			err = h.packageLocalDependency(dep, chartsDir)
		case ociRegistry.IsOCI(dep.Repository):
			ref := strings.TrimRight(dep.Repository, "/") + "/" + dep.Name
			slog.Debug("Downloading OCI dependency", "ref", ref, "version", dep.Version)

			err = downloadAndStandardize(ctx, downloader, ref, dep, chartsDir)

		default:
			err = h.downloadHTTPDependency(ctx, dep, chartsDir, &settings)
		}

		if err != nil {
			return fmt.Errorf("failed to download dependency %s-%s: %w", dep.Name, dep.Version, err)
		}

		slog.Debug("Dependency downloaded", "name", dep.Name, "version", dep.Version)
	}

	return nil
}

func (h *Client) chartDownloader(repoCred *resolvedCred) (*downloader.ChartDownloader, error) {
	settings := h.envSettings()

	registryClient, err := h.registryClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	var opts []getter.Option

	if repoCred != nil {
		if repoCred.Username != "" && repoCred.Password != "" {
			opts = append(opts, getter.WithBasicAuth(repoCred.Username, repoCred.Password))
		}

		if repoCred.certFile != "" && repoCred.keyFile != "" {
			opts = append(opts, getter.WithTLSClientConfig(repoCred.certFile, repoCred.keyFile, ""))
		}
	}

	return &downloader.ChartDownloader{
		Out:             os.Stderr,
		Verify:          downloader.VerifyNever,
		RepositoryCache: settings.RepositoryCache,
		RegistryClient:  registryClient,
		Getters:         getter.All(&settings),
		Options:         opts,
	}, nil
}

func (h *Client) credForURL(repoURL string) *resolvedCred {
	resolved := &resolvedCred{}

	if h.CredsStore == nil {
		return resolved
	}

	cred := h.CredsStore.ForURL(repoURL)
	if cred == nil {
		return resolved
	}

	resolved.Username = cred.Username
	resolved.Password = cred.Password

	if len(cred.TLSCert) > 0 && len(cred.TLSKey) > 0 {
		resolved.certFile = writeTempPEM(cred.TLSCert)
		resolved.keyFile = writeTempPEM(cred.TLSKey)
		resolved.cleanups = append(resolved.cleanups, resolved.certFile, resolved.keyFile)
	}

	return resolved
}

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

func (h *Client) downloadHTTPDependency(
	ctx context.Context, dep *chart.Dependency, chartsDir string, settings *helmCli.EnvSettings,
) error {
	repoCred := h.credForURL(dep.Repository)
	defer repoCred.cleanup()

	chartURL, err := repo.FindChartInRepoURL(
		dep.Repository, dep.Name, dep.Version,
		repoCred.certFile, repoCred.keyFile, "",
		getter.All(settings),
	)
	if err != nil {
		return fmt.Errorf("failed to resolve chart URL for %s: %w", dep.Name, err)
	}

	slog.Debug("Downloading HTTP dependency", "url", chartURL, "version", dep.Version)

	httpDownloader, err := h.chartDownloader(repoCred)
	if err != nil {
		return fmt.Errorf("failed to create authenticated downloader for %s: %w", dep.Name, err)
	}

	return downloadAndStandardize(ctx, httpDownloader, chartURL, dep, chartsDir)
}

func (h *Client) envSettings() helmCli.EnvSettings {
	return helmCli.EnvSettings{
		Debug:           h.Debug,
		RepositoryCache: getCacheDir(),
	}
}

func (h *Client) loginOCIRegistries(registryClient *ociRegistry.Client) {
	seen := make(map[string]struct{})

	for _, dep := range h.Chart.Metadata.Dependencies {
		if !ociRegistry.IsOCI(dep.Repository) {
			continue
		}

		host := extractOCIHost(dep.Repository)
		if _, ok := seen[host]; ok {
			continue
		}

		seen[host] = struct{}{}

		resolved := h.credForURL(dep.Repository)
		defer resolved.cleanup()

		opts := ociLoginOpts(resolved)
		if len(opts) == 0 {
			slog.Debug("Credential has no auth data, skipping login", "host", host)

			continue
		}

		slog.Debug("Logging in to OCI registry", "host", host)

		loginErr := registryClient.Login(host, opts...)
		if loginErr != nil {
			slog.Warn("Failed to login to OCI registry", "host", host, "err", loginErr)
		}
	}
}

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

func (h *Client) registryClient() (*ociRegistry.Client, error) {
	if h.cachedRegistryClient != nil {
		return h.cachedRegistryClient, nil
	}

	clientOpts := []ociRegistry.ClientOption{
		ociRegistry.ClientOptDebug(h.Debug),
		ociRegistry.ClientOptEnableCache(true),
	}

	if h.CredentialsFile != "" {
		slog.Debug("Using credentials file for OCI registry")

		clientOpts = append(clientOpts, ociRegistry.ClientOptCredentialsFile(h.CredentialsFile))
	}

	registryClient, err := ociRegistry.NewClient(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI registry client: %w", err)
	}

	if h.CredsStore != nil && h.Chart != nil {
		h.loginOCIRegistries(registryClient)
	}

	h.cachedRegistryClient = registryClient

	return registryClient, nil
}
