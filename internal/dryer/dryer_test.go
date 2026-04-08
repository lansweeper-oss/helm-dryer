package dryer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lansweeper-oss/helm-dryer/internal/dryer"
	client "github.com/lansweeper-oss/helm-dryer/internal/helm"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"helm.sh/helm/v3/pkg/chartutil"
)

const testFolder = "testdata/"

type expectedManifests struct {
	CRDs      int
	Hooks     int
	Manifests int
	Tests     int
}

var expectedFooBar = expectedManifests{
	CRDs:      2,
	Hooks:     1,
	Manifests: 11,
	Tests:     0,
}

var expectedHelloWorld = expectedManifests{
	CRDs:      0,
	Hooks:     2,
	Manifests: 5,
	Tests:     1,
}

// validate that we can also use yaml extension, if we can/want.
var (
	testFiles                 = []string{"values.yaml", "values.tpl.yaml", "values.stg.tpl.yaml"}
	testFilesInvalidSchema    = []string{"values.yaml", "values.invalidSchema.yaml"}
	testFilesTwoPass          = []string{"values.2pass.yaml", "values.2pass.tpl.yaml", "values.stg.tpl.yaml"}
	testFilesWithCustomDelims = []string{"values.yaml", "values.delims.tpl.yaml"}
	testFilesWithCapabilities = []string{"values.capabilities.tpl.yaml"}
	testSet                   = map[string]string{
		"clusterName":             "eks-cluster-platform",
		"domain":                  "test",
		"partition":               "aws",
		"accountId":               "234796234",
		"namePrefixWithoutDomain": "eks-cluster",
	}
)

func setupTest(t *testing.T, testFiles []string) *dryer.Input {
	t.Helper()

	setup := &dryer.Input{}
	setup.Data.Files = testFiles

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.yaml")
	require.NoError(t, err, "error creating temp file")

	setup.Settings.Out = tmpfile.Name()
	tmpfile.Close()

	tmpDir := t.TempDir()
	setup.Settings.Path = tmpDir

	err = os.CopyFS(tmpDir, os.DirFS(testFolder))
	require.NoError(t, err, "error copying test data")

	setup.Data.ReleaseName = os.Getenv("ARGOCD_APP_NAME")

	if setup.Data.ReleaseName == "" {
		setup.Data.ReleaseName = "test-template-chart"
	}

	setup.Data.ReleaseNamespace = os.Getenv("ARGOCD_APP_NAMESPACE")

	if setup.Data.ReleaseNamespace == "" {
		setup.Data.ReleaseNamespace = "test"
	}

	setup.Data.APIVersions = []string{"monitoring.coreos.com/v1/PrometheusRule"}
	setup.Data.KubeVersion = "1.33"

	setup.Data.Set = map[string]string{}
	for k := range testSet {
		setup.Data.Set[k] = testSet[k]
	}

	setup.Settings.UpdateDependencies = false
	setup.Settings.Logging.Debug = true
	setup.Settings.Logging.Format = "text"

	return setup
}

func TestTemplateValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	expected, err := utils.ParseYAMLFile(filepath.Join(test.Settings.Path, "values.expected.yaml"))
	require.NoError(t, err, "Failed to parse expected values file")

	// Set values are transparently passed (TODO: make this optional with a flag?)
	for key, value := range test.Data.Set {
		expected[key] = value
	}

	assert.Equal(t, expected, out, "The rendered values are different from the expected ones")

	// Now test again with raw values files, out from a chart folder.
	tempDir := t.TempDir()

	for _, file := range test.Data.Files {
		err := utils.CopyFile(
			filepath.Join(test.Settings.Path, file),
			filepath.Join(tempDir, file),
		)
		require.NoError(t, err, "Error copying file to temp directory")
	}

	test.Settings.Path = tempDir
	err = test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error for raw file")
}

func TestTemplateWithCustomDelims(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesWithCustomDelims)

	test.Settings.DelimLeft = "<<"
	test.Settings.DelimRight = ">>"

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	expected, err := utils.ParseYAMLFile(filepath.Join(test.Settings.Path, "values.expected.yaml"))
	require.NoError(t, err, "Failed to parse expected values file")

	// Set values are transparently passed (TODO: make this optional with a flag?)
	for key, value := range test.Data.Set {
		expected[key] = value
	}

	assert.Equal(t, expected, out, "The rendered values are different from the expected ones")

	// Now test again with raw values files, out from a chart folder.
	tempDir := t.TempDir()

	for _, file := range test.Data.Files {
		err := utils.CopyFile(
			filepath.Join(test.Settings.Path, file),
			filepath.Join(tempDir, file),
		)
		require.NoError(t, err, "Error copying file to temp directory")
	}

	test.Settings.Path = tempDir
	err = test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error for raw file")
}

func TestTemplateCapabilities(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesWithCapabilities)

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	expected, err := utils.ParseYAMLFile(filepath.Join(test.Settings.Path, "values.capabilities.expected.yaml"))
	require.NoError(t, err, "Failed to parse expected capabilities values file")
	// Set values are transparently passed (TODO: make this optional with a flag?)
	for key, value := range test.Data.Set {
		expected[key] = value
	}

	assert.Equal(t, expected, out, "The rendered values are different from the expected ones")
}

func Test2PassTemplateValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesTwoPass)

	test.Settings.TwoPass = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues` should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	expected, err := utils.ParseYAMLFile(filepath.Join(test.Settings.Path, "values.2pass.expected.yaml"))
	require.NoError(t, err, "Failed to parse expected 2-pass values file")
	// Set values are transparently passed (TODO: make this optional with a flag?)
	for key, value := range test.Data.Set {
		expected[key] = value
	}

	assert.Equal(t, expected, out, "The rendered values are different from the expected ones")
}

func TestTemplateChart(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["tags.foobar"] = "true"
	test.Settings.SkipCRDs = true

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")
		assert.NotEmpty(t, data, "The rendered chart should not be empty")
	}

	msg := fmt.Sprintf("The rendered chart should have %d items", expectedFooBar.Manifests)
	assert.Equal(
		t,
		expectedFooBar.Manifests,
		numManifests,
		msg,
	)
}

func TestSkipSchema(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesInvalidSchema)

	err := test.TemplateChart(context.Background())
	require.Error(t, err, "TemplateChart should return an error when SkipSchemaValidation=False")

	test.Settings.SkipSchemaValidation = true
	err = test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error when SkipSchemaValidation=True")
}

func TestSkipTestHooks(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["tags.hello"] = "true"
	test.Settings.SkipTests = true

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")
		assert.NotEmpty(t, data, "The rendered chart should not be empty")
	}

	msg := fmt.Sprintf("The rendered chart should have %d items", expectedHelloWorld.Manifests)
	assert.Equal(
		t,
		expectedHelloWorld.Manifests-expectedHelloWorld.Tests,
		numManifests,
		msg,
	)
}

func TestNullRemovesKeys(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	merged, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	options := chartutil.ReleaseOptions{
		Name:      test.Data.ReleaseName,
		Namespace: test.Data.ReleaseNamespace,
		Revision:  1,
		IsInstall: true,
		IsUpgrade: false,
	}
	helmClient := client.Client{Path: test.Settings.Path, Debug: true}
	err = helmClient.LoadChart()

	require.NoError(t, err, "Failed to load chart")

	chart := helmClient.Chart

	// Replicate the issue where we used to omit chart.Values
	chartValues, err := utils.DeepCopy(chart.Values)
	require.NoError(t, err, "Failed to process chart values")

	chart.Values = map[string]any{}
	result, err := chartutil.ToRenderValues(chart, merged, options, chartutil.DefaultCapabilities)
	require.NoError(t, err, "Error obtaining final values")

	finalValues := result["Values"].(chartutil.Values)["monitoring"].(map[string]any)
	_, exists := finalValues["service"].(map[string]any)["name"]
	assert.True(t, exists, "The 'service.name' key is passed as null when we omit chart values")

	// restore chart.Values and try again; Helm 3.20.1+ preserves null values from chart defaults.
	chart.Values = chartValues
	result, err = chartutil.ToRenderValues(chart, merged, options, chartutil.DefaultCapabilities)
	require.NoError(t, err, "Error obtaining final values")

	finalValues = result["Values"].(chartutil.Values)["monitoring"].(map[string]any)
	_, exists = finalValues["service"].(map[string]any)["name"]
	assert.True(t, exists, "The 'service.name' key should not exist when we pass chart values")

	// Test that user-provided null values override non-null chart defaults and remove the key
	chart.Values = map[string]any{
		"monitoring": map[string]any{
			"service": map[string]any{
				"name": "chart-default-name",
			},
		},
	}
	mergedWithUserNull := map[string]any{
		"monitoring": map[string]any{
			"service": map[string]any{
				"name": nil, // User explicitly nullifies the chart default
			},
		},
	}
	result, err = chartutil.ToRenderValues(chart, mergedWithUserNull, options, chartutil.DefaultCapabilities)
	require.NoError(t, err, "Error obtaining final values")

	finalValues = result["Values"].(chartutil.Values)["monitoring"].(map[string]any)
	_, exists = finalValues["service"].(map[string]any)["name"]
	assert.False(t, exists, "User-provided null should override and remove the chart's non-null value")
}

// TestIgnoreMainValues validates that null values in templated values files are handled gracefully.
// When IgnoreMainValues=true, main values are omitted and only templated values are used.
// The templates should handle null values gracefully (e.g., by using default fallbacks) rather than fail.
// For example, when we have:
//
// [values.yaml]
// foo: bar
// [values.tpl.yaml]
// foo: null
//
// The template should render successfully with null values converted to sensible defaults.
// This is the behavior introduced in Helm 3.20.1 where null values are preserved and handled gracefully.
func TestIgnoreMainValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["tags.hello"] = "true"

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error")

	// Use only templated values which contain null values; templates should handle these gracefully
	test.Settings.IgnoreMainValues = true
	err = test.TemplateChart(context.Background())

	require.NoError(t, err, "TemplateChart should succeed with null values handled gracefully by templates")
}

func TestUsingFolderAsOutput(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["tags.hello"] = "true"
	test.Settings.Out = t.TempDir()

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error")

	files, err := os.ReadDir(filepath.Join(test.Settings.Out, "test-chart", "charts", "hello-world", "templates"))
	require.NoError(t, err, "Cannot read output folder")

	// Writing to a folder ignores hooks (and tests).
	assert.Len(
		t,
		files,
		expectedHelloWorld.Manifests-expectedHelloWorld.Hooks,
		"The output folder should contain one file per manifest and exclude hooks",
	)
}

func TestRenderChartAsCMP(t *testing.T) {
	releaseName := "overridden-release-name"
	releaseNamespace := "overridden-release-namespace"
	// Validate that the app name and namespace can be obtained from environment variables
	t.Setenv("ARGOCD_APP_NAME", releaseName)
	t.Setenv("ARGOCD_APP_NAMESPACE", releaseNamespace)

	test := setupTest(t, testFiles)

	test.Data.Set["tags.hello"] = "true"

	testFilesAsJSON, _ := json.Marshal(testFiles)
	testSetAsJSON, _ := json.Marshal(test.Data.Set)

	// set ARGOCD_APP_PARAMETERS environment variable to simulate the CMP parameters
	t.Setenv(
		"ARGOCD_APP_PARAMETERS",
		`[
			{
				"name":"valueFiles",
				"array": `+string(testFilesAsJSON)+`
			},
			{
				"name":"valuesObject",
				"map": `+string(testSetAsJSON)+`
			}
		]`,
	)

	err := test.RenderChart(context.Background())
	require.NoError(t, err, "RenderChart should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")
		assert.NotEmpty(t, data, "The rendered chart should not be empty")

		if _, ok := data["metadata"].(map[string]any)["namespace"]; ok {
			assert.Equal(
				t,
				releaseNamespace, data["metadata"].(map[string]any)["namespace"],
				"The release namespace should be overridden",
			)
		}

		if data["kind"] == "PrometheusRule" {
			assert.Equal(
				t,
				releaseName,
				data["metadata"].(map[string]any)["labels"].(map[string]any)["app.kubernetes.io/instance"],
				"The release name should be overridden",
			)
		}
	}

	msg := fmt.Sprintf("The rendered chart should have %d items", expectedHelloWorld.Manifests)
	assert.Equal(
		t,
		expectedHelloWorld.Manifests,
		numManifests,
		msg,
	)
}

func TestTwoPassRenderChartAsCMP(t *testing.T) {
	test := setupTest(t, testFilesTwoPass)

	test.Data.Set["tags.foobar"] = "true"
	test.Settings.IgnoreMainValues = true
	test.Settings.TwoPass = true
	testFilesAsJSON, _ := json.Marshal(testFilesTwoPass)
	testSetAsJSON, _ := json.Marshal(test.Data.Set)

	// set ARGOCD_APP_PARAMETERS environment variable to simulate CMP parameters
	t.Setenv(
		"ARGOCD_APP_PARAMETERS",
		`[
			{
				"name":"valueFiles",
				"array": `+string(testFilesAsJSON)+`
			},
			{
				"name":"valuesObject",
				"map": `+string(testSetAsJSON)+`
			},
			{
				"name":"settings",
				"map":{
					"skipCRDs":"true",
					"ttl":"10m0s"
				}
			}
		]`,
	)

	err := test.RenderChart(context.Background())
	require.NoError(t, err, "RenderChart should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")
		assert.NotEmpty(t, data, "The rendered chart should not be empty")
	}

	msg := fmt.Sprintf("The rendered chart should have %d items", expectedFooBar.Manifests)
	assert.Equal(
		t,
		expectedFooBar.Manifests,
		numManifests,
		msg,
	)
}

func TestTwoPassWithDependencies(t *testing.T) {
	// This time instruct this is a 2-pass render from the input settings.
	test := setupTest(t, testFilesTwoPass)

	test.Data.Set["tags.hello"] = "true"
	test.Settings.IgnoreMainValues = true
	test.Settings.TwoPass = true

	testFilesTwoPassAsJSON, _ := json.Marshal(testFilesTwoPass)
	testSetAsJSON, _ := json.Marshal(test.Data.Set)

	// set ARGOCD_APP_PARAMETERS environment variable to simulate CMP parameters
	t.Setenv(
		"ARGOCD_APP_PARAMETERS",
		`[
			{
				"name":"valueFiles",
				"array": `+string(testFilesTwoPassAsJSON)+`
			},
			{
				"name":"valuesObject",
				"map": `+string(testSetAsJSON)+`
			},
			{
				"name":"settings",
				"map":{
					"skipCRDs":"true",
					"twoPass":"true"
				}
			}
		]`,
	)

	err := test.RenderChart(context.Background())
	require.NoError(t, err, "RenderChart should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")
		assert.NotEmpty(t, data, "The rendered chart should not be empty")

		if data["kind"] == "PrometheusRule" {
			yamlFile, err := os.Open(filepath.Join(test.Settings.Path, "prometheus-rule.expected.yaml"))
			require.NoError(t, err, "Cannot open expected PrometheusRule file")

			defer yamlFile.Close()

			decoder := yaml.NewDecoder(yamlFile)
			expected := make(map[string]any)
			err = decoder.Decode(&expected)
			require.NoError(t, err, "Error decoding expected PrometheusRule YAML")
			assert.Equal(t, expected, data, "The rendered file is different from the expected one")
		}
	}

	msg := fmt.Sprintf("The rendered chart should have %d items", expectedHelloWorld.Manifests)
	assert.Equal(
		t,
		expectedHelloWorld.Manifests,
		numManifests,
		msg,
	)
}

func TestRenderFromApp(t *testing.T) {
	t.Parallel()

	releaseName := "overridden-release-name"
	releaseNamespace := "test"

	test := setupTest(t, testFiles)

	// Create a temporary ArgoCD Application spec file
	appSpec := `
spec:
  ignoreMe: of-course
  project: test-project
  source:
    path: .
    plugin:
      parameters:
        - name: settings
          map:
            disableHooks: "true"
            releaseName: ` + releaseName + `
            releaseNamespace: ` + releaseNamespace + `
        - name: valueFiles
          array:
            - values.yaml
            - values.tpl.yaml
            - values.stg.tpl.yaml
        - name: valuesObject
          map:
            accountId: 234796234
            clusterName: eks-cluster-platform
            domain: test
            namePrefixWithoutDomain: eks-cluster
            partition: aws
            tags.foobar: ""
  destination:
    name: test-template-chart
    namespace: overridden-from-parameter
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "app-spec-*.yaml")
	if err != nil {
		t.Fatalf("error creating temp file: %v", err)
	}

	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.WriteString(appSpec)
	if err != nil {
		t.Fatalf("error writing to temp file: %v", err)
	}

	tmpfile.Close()

	// Set the application spec file in the CLI and do an override
	test.AppSettings.ApplicationSpec = tmpfile.Name()

	err = test.RenderFromApp(context.Background())
	require.NoError(t, err, "RenderFromApp should not return an error")

	yamlFile, err := os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file")

	dec := yaml.NewDecoder(bytes.NewReader(yamlFile))
	expectedManifests := 10
	expectedCRDs := 2
	numManifests := 0

	for {
		var data map[string]any

		err := dec.Decode(&data)
		if errors.Is(err, io.EOF) {
			break
		}

		numManifests++

		require.NoError(t, err, "Error decoding YAML")

		assert.NotEmpty(t, data, "The rendered chart should not be empty")

		if _, ok := data["metadata"].(map[string]any)["namespace"]; ok {
			assert.Equal(
				t,
				releaseNamespace, data["metadata"].(map[string]any)["namespace"],
				"The release namespace should be overridden",
			)
		}

		if data["kind"] == "PrometheusRule" {
			assert.Equal(
				t,
				releaseName,
				data["metadata"].(map[string]any)["labels"].(map[string]any)["app.kubernetes.io/instance"],
				"The release name should be overridden",
			)
		}
	}

	_, err = os.ReadFile(test.Settings.Out)
	require.NoError(t, err, "Cannot read output file again for printing")

	msg := fmt.Sprintf(
		"The rendered chart should have %d resources and %d CRDs (%d manifests) but obtained %d manifests instead.",
		expectedManifests,
		expectedCRDs,
		expectedCRDs+expectedManifests,
		numManifests,
	)
	assert.Equal(
		t,
		expectedCRDs+expectedManifests,
		numManifests,
		msg,
	)
}

func TestSetOverridesFiles(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["foo-bar.logLevel"] = "debug"
	test.Data.Set["foo-bar.serviceMonitor.enabled"] = "false"

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	controllerValues, ok := out["foo-bar"].(map[string]any)
	require.True(t, ok, "Error processing dot notation values")
	// Cannot check as boolean due to the limitation of c.Set being map[string]string
	assert.Equal(
		t,
		"false",
		controllerValues["serviceMonitor"].(map[string]any)["enabled"],
		"Set should override the file values",
	)
	assert.Equal(
		t,
		"debug",
		controllerValues["logLevel"],
		"Set should override the file values",
	)
}

func TestIncorrectOutputFallback(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Settings.Out = "/non/existing/folder"
	assert.False(t, test.UsingFolderAsOutput(), "UsingFolderAsOutput should return false for a non-existing folder")
	assert.Empty(t, test.Settings.Out, "The output setting should be reset to empty string when the folder does not exist")
}
