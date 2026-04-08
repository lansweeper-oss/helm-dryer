package dryer

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	client "github.com/lansweeper-oss/helm-dryer/internal/helm"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
)

// UsingFolderAsOutput checks if the output setting is a folder and resets to stdout if the output is invalid.
func (in *Input) UsingFolderAsOutput() bool {
	outputToFolder, err := utils.IsDir(in.Settings.Out)
	if err != nil {
		slog.Warn("Could not determine if output is a folder, falling back to stdout", "error", err)

		in.Settings.Out = ""
	}

	return outputToFolder
}

// RenderChart renders the chart using the provided values and settings.
// It reads the values files, merges them with the provided values, and
// then renders the chart using the Helm client.
// The rendered chart is written to the specified output file or to stdout if no output file is specified.
func (in *Input) RenderChart(ctx context.Context) error {
	// Override from parameters
	err := in.ReadEnvironment()
	if err != nil {
		return fmt.Errorf("error reading environment: %w", err)
	}

	return in.TemplateChart(ctx)
}

// TemplateChart renders the chart using the provided values and settings.
// It reads the values files, merges them with the provided values, and
// then renders the chart using the Helm client.
func (in *Input) TemplateChart(ctx context.Context) error {
	helmClient := client.Client{
		Credentials:        &in.Settings.Credentials,
		Debug:              in.Settings.Logging.Debug,
		Path:               in.Settings.Path,
		TTL:                utils.GetTTL(in.Settings.TTL),
		UpdateDependencies: in.Settings.UpdateDependencies,
	}

	vals, err := helmClient.ReadChartDependencies(ctx)
	if err != nil {
		return fmt.Errorf("failed to read chart dependencies: %w", err)
	}

	merged, err := in.compoundValues(vals)
	if err != nil {
		return fmt.Errorf("error templating values: %w", err)
	}

	slog.Debug("Chart values overridden with", "values", merged)

	return in.renderChart(merged)
}

func (in *Input) renderChart(vals map[string]any) error {
	folderOutput := in.UsingFolderAsOutput()

	chartLoader, err := loader.Loader(in.Settings.Path)
	if err != nil {
		return fmt.Errorf("failed to initialize chart loader: %w", err)
	}

	chart, err := chartLoader.Load()
	if err != nil {
		return fmt.Errorf("failed to load Chart: %w", err)
	}

	helmClient, err := in.buildHelmClient(folderOutput)
	if err != nil {
		return fmt.Errorf("failed to build helm client: %w", err)
	}

	// Helm treats differently those values that are "Chart defaults" and those which are injected
	// or user-defined, especially when it comes to nullified values.
	// This is an edge case where values not targeting a subchart are expected (or not) to be
	// removed from the final value set if set to nil.
	// With the default Helm behavior, values.yaml is loaded implicitly, which may be confusing.
	if in.Settings.IgnoreMainValues {
		chart.Values = map[string]any{}
	}

	rel, err := helmClient.Run(chart, vals)
	if err != nil {
		return fmt.Errorf("could not render helm chart correctly: %w", err)
	}

	// If output is a folder, hooks and tests are already ignored (not written to a file).
	if folderOutput {
		return nil
	}

	manifests := rel.Manifest

	if in.Settings.SkipTests {
		in.skipTestResources(rel)
	}

	if !in.AppSettings.DisableHooks {
		var builder strings.Builder
		builder.WriteString(manifests)

		for _, hook := range rel.Hooks {
			builder.WriteString("\n---\n# Source: ")
			builder.WriteString(hook.Path)
			builder.WriteByte('\n')
			builder.WriteString(hook.Manifest)
		}

		manifests = builder.String()
	}

	err = utils.WriteOutput([]byte(manifests), in.Settings.Out)
	if err != nil {
		return fmt.Errorf("failed to write manifests: %w", err)
	}

	return nil
}

// Creates a helm client and configures it according to the input settings.
// The client is delivered prepared to run a local dry-run "install" operation.
func (in *Input) buildHelmClient(folderOutput bool) (*action.Install, error) {
	helmClient := action.NewInstall(&action.Configuration{})
	helmClient.ClientOnly = true
	helmClient.DryRun = true
	helmClient.IncludeCRDs = !in.Settings.SkipCRDs
	helmClient.SkipCRDs = in.Settings.SkipCRDs
	helmClient.SkipSchemaValidation = in.Settings.SkipSchemaValidation

	if len(in.Data.APIVersions) > 0 {
		helmClient.APIVersions = in.Data.APIVersions
	}

	if in.Data.KubeVersion != "" {
		kubeVersion, err := chartutil.ParseKubeVersion(in.Data.KubeVersion)
		if err != nil {
			return nil, fmt.Errorf("error parsing Kubernetes version: %w", err)
		}

		helmClient.KubeVersion = kubeVersion
	}

	helmClient.Namespace = in.Data.ReleaseNamespace
	helmClient.ReleaseName = in.Data.ReleaseName

	if folderOutput {
		helmClient.OutputDir = in.Settings.Out
	}

	return helmClient, nil
}

// Remove resources containing "helm.sh/hook: test".
// These are stored as Hooks inside the release instead as Manifests.
func (in *Input) skipTestResources(rel *release.Release) {
	rel.Hooks = slices.DeleteFunc(rel.Hooks, func(hook *release.Hook) bool {
		return slices.Contains(hook.Events, release.HookTest)
	})
}

func (in *Input) templateYAMLFile(
	file string, vals, runtime map[string]any,
) (map[string]any, error) {
	var option string
	if in.Settings.TwoPass || in.Settings.IgnoreEmpty {
		option = "missingkey=default"
	} else {
		option = "missingkey=error"
	}

	templateOptions := client.Options{
		DelimLeft:       in.Settings.DelimLeft,
		DelimRight:      in.Settings.DelimRight,
		TemplateOptions: option,
	}

	result, err := client.TemplateAndParseYaml(file, templateOptions, vals, runtime)
	if err != nil {
		return nil, fmt.Errorf("error templating file %s: %w", file, err)
	}

	return result, nil
}
