// Package dryer provides functions to read and process templated Helm values files.
package dryer

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lansweeper-oss/helm-dryer/internal/argo"
	"github.com/lansweeper-oss/helm-dryer/internal/cli"
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	client "github.com/lansweeper-oss/helm-dryer/internal/helm"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/lansweeper-oss/helm-dryer/internal/values"
	"go.yaml.in/yaml/v3"
)

type Input struct {
	AppSettings cli.AppSettings
	Data        cli.Data
	Settings    cli.Settings
}

var kubeVersionPattern = regexp.MustCompile(`^\d+\.\d+(\..+)?$`)

// RenderFromApp renders an application taking the input values from an ArgoCD Application file.
// Values preference is a bit different from the other commands:
//  1. The CLI settings are read from the command line (low preference except for path).
//  2. Environment variables are ignored.
//  3. The values files are read and merged with the application spec (high preference).
//  4. When CLI sets a custom path (rather than current folder), it assumes the one from the
//     Application spec is relative to the one set as a parameter.
//     This is useful for CI, where the path may be set differently when comparing HEAD with the
//     main branch.
func (in *Input) RenderFromApp() error {
	content, err := os.ReadFile(in.AppSettings.ApplicationSpec)
	if err != nil {
		return fmt.Errorf("failed to read application spec file %s: %w", in.AppSettings.ApplicationSpec, err)
	}

	var app argo.App

	err = yaml.Unmarshal(content, &app)
	if err != nil {
		return fmt.Errorf("failed to unmarshal application spec file %s: %w", in.AppSettings.ApplicationSpec, err)
	}

	slog.Debug("Successfully read application spec", "app", app)

	// application path is relative to the one set in the CLI
	in.Settings.Path = filepath.Join(in.Settings.Path, app.Spec.Source.Path)

	if in.Data.ReleaseName == "" && app.Metadata.Name != "" {
		slog.Debug("Setting release name from the application metadata.name field")

		in.Data.ReleaseName = app.Metadata.Name
	}

	if in.Data.ReleaseNamespace == "" && app.Spec.Destination.Namespace != "" {
		slog.Debug("Setting release namespace from the application spec.destination field")

		in.Data.ReleaseNamespace = app.Spec.Destination.Namespace
	}

	// Override from parameters
	in.ReadParameters(app.Spec.Source.Plugin.Parameters)

	err = in.TemplateChart()
	if err != nil {
		return fmt.Errorf("error rendering application: %w", err)
	}

	return nil
}

// TemplateValues generates a YAML file with the merged values from the provided files and the set values.
// It writes the output to the specified file or to stdout if no output file is specified.
func (in *Input) TemplateValues() error {
	vals := make(map[string]any)
	chartFilePath := filepath.Join(in.Settings.Path, "Chart.yaml")

	_, err := os.Stat(chartFilePath)
	if !errors.Is(err, os.ErrNotExist) {
		helmClient := client.Client{
			Credentials:        &in.Settings.Credentials,
			Debug:              in.Settings.Logging.Debug,
			Path:               in.Settings.Path,
			TTL:                utils.GetTTL(in.Settings.TTL),
			UpdateDependencies: in.Settings.UpdateDependencies,
		}

		if err != nil {
			slog.Warn("Unexpected error checking Chart.yaml", "path", chartFilePath, "error", err)
		}

		vals, err = helmClient.ReadChartDependencies()
		if err != nil {
			return fmt.Errorf("failed to read chart dependencies: %w", err)
		}
	}

	merged, err := in.compoundValues(vals)
	if err != nil {
		return fmt.Errorf("error merging values: %w", err)
	}

	yamlBytes, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("error marshalling YAML: %w", err)
	}

	err = utils.WriteOutput(yamlBytes, in.Settings.Out)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// compoundValues merges the values files in a resulting map[string]any.
//
// We don't want to return the Chart dependencies values in the merged values, since this would
// be too noisy and not useful for an end user. We use those dependency values though to feed
// the templating engine, so that the values files can use the dependencies as well.
func (in *Input) compoundValues(initialValues map[string]any) (map[string]any, error) {
	cliVals, err := values.DotNotationToMap(in.Data.Set)
	if err != nil {
		return nil, fmt.Errorf("error converting CLI set values to map: %w", err)
	}

	vals, err := utils.DeepCopy(cliVals)
	if err != nil {
		return nil, fmt.Errorf("error copying of CLI values: %w", err)
	}

	runtimeValues, err := in.runtimeValues()
	if err != nil {
		return nil, fmt.Errorf("error collecting runtime values: %w", err)
	}

	err = values.MergeYamlMaps(vals, initialValues)
	if err != nil {
		return nil, fmt.Errorf("error merging initial values: %w", err)
	}

	processed, err := in.processValuesFiles(vals, runtimeValues, cliVals)
	if err != nil {
		return nil, fmt.Errorf("error merging YAML data: %w", err)
	}

	if in.Settings.TwoPass {
		// NOTE: TwoPass is set to false to prevent infinite recursion and to switch the second pass
		// to missingkey=error mode (via templateYAMLFile). This permanently mutates the Input, which
		// is acceptable because TwoPass is consumed exactly once per TemplateValues/TemplateChart call.
		in.Settings.TwoPass = false
		// merge again over the initial values to ensure they are not lost
		err = values.MergeYamlMaps(vals, processed)
		if err != nil {
			return nil, fmt.Errorf("error merging initial values: %w", err)
		}

		return in.compoundValues(vals)
	}

	return processed, nil
}

// processValuesFiles goes through all values files and returns templated data and default values.
func (in *Input) processValuesFiles(
	initialValues, runtimeValues, cliVals map[string]any,
) (map[string]any, error) {
	// +1 to accommodate the values Object (in.Data.Set) at the end of the slice
	templatedData := make([]map[string]any, 0, len(in.Data.Files)+1)

	for _, file := range in.Data.Files {
		fileWithPath := filepath.Join(in.Settings.Path, file)

		_, err := os.Stat(fileWithPath)
		if errors.Is(err, os.ErrNotExist) {
			if in.Settings.IgnoreMissing {
				slog.Debug("Ignoring missing file: " + file)

				continue
			}

			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, file)
		} else if err != nil {
			return nil, fmt.Errorf("failed to stat values file %s: %w", file, err)
		}

		slog.Debug("Reading values file: " + fileWithPath)

		data, err := in.templateYAMLFile(fileWithPath, initialValues, runtimeValues)
		if err != nil {
			return nil, fmt.Errorf("error reading values file %s: %w", fileWithPath, err)
		}

		templatedData = append(templatedData, data)
	}

	// Finally add the values from cli.Set (valuesObject)
	templatedData = append(templatedData, cliVals)

	merged, err := values.MergeYAMLArrayOfMaps(templatedData)
	if err != nil {
		return nil, fmt.Errorf("error merging YAML data: %w", err)
	}

	return merged, nil
}

// runtimeValues collects the runtime values (mainly .Release and .Capabilities).
func (in *Input) runtimeValues() (map[string]any, error) {
	major, minor := "", ""

	if in.Data.KubeVersion != "" {
		matched := kubeVersionPattern.MatchString(in.Data.KubeVersion)

		if !matched {
			return nil, fmt.Errorf("%w: %s", dryerr.ErrInvalidKubeVersionFormat, in.Data.KubeVersion)
		}

		kubeVersion := strings.Split(in.Data.KubeVersion, ".")
		major = kubeVersion[0]
		minor = kubeVersion[1]
	}

	runtimeValues := map[string]any{
		"Release": map[string]any{
			"Name":      in.Data.ReleaseName,
			"Namespace": in.Data.ReleaseNamespace,
		},
		"Capabilities": map[string]any{
			"APIVersions": in.Data.APIVersions,
			"KubeVersion": map[string]any{
				"Major":   major,
				"Minor":   minor,
				"Version": in.Data.KubeVersion,
			},
		},
	}

	return runtimeValues, nil
}
