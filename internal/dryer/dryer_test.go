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
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	client "github.com/lansweeper-oss/helm-dryer/internal/helm"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
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
	setup.Settings.StripNullValues = true
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

	options := common.ReleaseOptions{
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
	result, err := chartutil.ToRenderValues(chart, merged, options, common.DefaultCapabilities)
	require.NoError(t, err, "Error obtaining final values")

	finalValues := result["Values"].(common.Values)["monitoring"].(map[string]any)
	_, exists := finalValues["service"].(map[string]any)["name"]
	assert.True(t, exists, "The 'service.name' key is passed as null when we omit chart values")

	// restore chart.Values and try again; we control this behavior with Settings.PreserveNullKeys.
	chart.Values = chartValues
	result, err = chartutil.ToRenderValues(chart, merged, options, common.DefaultCapabilities)
	require.NoError(t, err, "Error obtaining final values")

	finalValues = result["Values"].(common.Values)["monitoring"].(map[string]any)
	nameVal := finalValues["service"].(map[string]any)["name"]
	assert.Nil(t, nameVal, "The 'service.name' key should be nil when we pass chart values")
}

// TestIgnoreMainValues validates the edge case of nullified values and umbrella charts, where
// values targeting templates rather than subcharts are expected (or not) to be removed.
// For example, when we have:
//
// [values.yaml]
// foo: bar
// [values.tpl.yaml]
// foo: null
//
// and foo is not a subchart (otherwise, subchart's values apply), we might expect the key to be
// removed as in https://helm.sh/docs/chart_template_guide/values_files/#deleting-a-default-key.
//
// NOTE: Helm v4 changed nil value coalescing — nils are preserved in both modes, so
// IgnoreMainValues no longer causes template failures for nil values. Charts should
// use `| default ""` to handle nil values defensively.
func TestIgnoreMainValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Data.Set["tags.hello"] = "true"

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart should not return an error")

	// Preserve the null values (that is, we omit values.yaml implicitly load while keeping it from Data.Files)
	test.Settings.IgnoreMainValues = true
	err = test.TemplateChart(context.Background())

	require.NoError(t, err, "TemplateChart should not return an error with IgnoreMainValues in Helm v4")
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

func TestInitialValuesAsCMP(t *testing.T) {
	t.Setenv("ARGOCD_APP_NAME", "test-release")
	t.Setenv("ARGOCD_APP_NAMESPACE", "test")

	test := setupTest(t, testFiles)

	// Write an initial values file with the required set values as nested YAML
	ivFile := filepath.Join(test.Settings.Path, "initial.yaml")

	ivVals := make(map[string]any, len(testSet))
	for k, v := range testSet {
		ivVals[k] = v
	}

	ivVals["tags"] = map[string]any{"hello": "true"}

	ivBytes, err := yaml.Marshal(ivVals)
	require.NoError(t, err, "error marshalling initial values")

	err = os.WriteFile(ivFile, ivBytes, 0o644)
	require.NoError(t, err, "error writing initial values file")

	testFilesAsJSON, _ := json.Marshal(testFiles)
	ivFilesAsJSON, _ := json.Marshal([]string{"initial.yaml"})

	t.Setenv(
		"ARGOCD_APP_PARAMETERS",
		`[
			{
				"name":"initialValues",
				"array": `+string(ivFilesAsJSON)+`
			},
			{
				"name":"valueFiles",
				"array": `+string(testFilesAsJSON)+`
			}
		]`,
	)

	err = test.RenderChart(context.Background())
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

// writeMarkerValues writes env/prod.yaml under base, holding a marker to tell the file apart.
func writeMarkerValues(t *testing.T, base, marker string) {
	t.Helper()

	err := os.MkdirAll(filepath.Join(base, "env"), utils.ReadWriteDir)
	require.NoError(t, err, "error creating the env folder")

	err = os.WriteFile(
		filepath.Join(base, "env", "prod.yaml"),
		[]byte("absoluteMarker: "+marker+"\n"),
		utils.ReadWrite,
	)
	require.NoError(t, err, "error writing the marker values file")
}

// setupRepoTest lays the chart out in a repository-like folder, with the chart in a subfolder,
// a values file at the root of the repository and a decoy with the same relative path under the
// chart folder, so we can tell from which base an absolute values file was resolved.
func setupRepoTest(t *testing.T, files []string) *dryer.Input {
	t.Helper()

	test := setupTest(t, files)

	repoRoot := t.TempDir()
	chartPath := filepath.Join(repoRoot, "charts", "app")

	err := os.MkdirAll(chartPath, utils.ReadWriteDir)
	require.NoError(t, err, "error creating the chart folder")

	err = os.CopyFS(chartPath, os.DirFS(testFolder))
	require.NoError(t, err, "error copying test data")

	writeMarkerValues(t, repoRoot, "from-repo-root")
	writeMarkerValues(t, chartPath, "from-chart-path")

	test.Settings.Path = chartPath
	test.Settings.RepoRoot = repoRoot

	return test
}

func TestAbsoluteValuesFileResolvesFromRepoRoot(t *testing.T) {
	t.Parallel()

	test := setupRepoTest(t, []string{"values.yaml", "/env/prod.yaml"})

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	assert.Equal(
		t,
		"from-repo-root",
		out["absoluteMarker"],
		"An absolute values file should be resolved from the repository root, not from the chart path",
	)
}

func TestAbsoluteValuesFileOutsideRepoRoot(t *testing.T) {
	t.Parallel()

	test := setupRepoTest(t, []string{"values.yaml", "/../env/prod.yaml"})

	// A path escaping the repository root is a misconfiguration rather than a missing file.
	test.Settings.IgnoreMissing = true

	err := test.TemplateValues(context.Background())
	require.ErrorIs(t, err, dryerr.ErrOutsideRepoRoot, "Values files outside the repository root should be rejected")
}

func TestInitialValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	ivFile := filepath.Join(test.Settings.Path, "initial.yaml")
	ivContent := "foo-bar:\n  logLevel: debug\n  serviceMonitor:\n    enabled: \"false\"\n"
	err := os.WriteFile(ivFile, []byte(ivContent), 0o644)
	require.NoError(t, err, "error writing initial values file")

	test.Data.InitialValues = []string{"initial.yaml"}

	err = test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	controllerValues, ok := out["foo-bar"].(map[string]any)
	require.True(t, ok, "Expected nested foo-bar key from initial values file")
	assert.Equal(t, "false", controllerValues["serviceMonitor"].(map[string]any)["enabled"])
	assert.Equal(t, "debug", controllerValues["logLevel"])
}

func TestInitialValuesMultiple(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	file1 := filepath.Join(test.Settings.Path, "vo1.yaml")
	file2 := filepath.Join(test.Settings.Path, "vo2.yaml")

	err := os.WriteFile(file1, []byte("foo-bar:\n  logLevel: info\n"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(file2, []byte("foo-bar:\n  logLevel: debug\n  serviceMonitor:\n    enabled: \"false\"\n"), 0o644)
	require.NoError(t, err)

	test.Data.InitialValues = []string{"vo1.yaml", "vo2.yaml"}

	err = test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err)

	controllerValues, ok := out["foo-bar"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "false", controllerValues["serviceMonitor"].(map[string]any)["enabled"])
	assert.Equal(t, "debug", controllerValues["logLevel"], "Last file should win over earlier files")
}

func TestInitialValuesCLITakesPrecedence(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	voFile := filepath.Join(test.Settings.Path, "values-object.yaml")
	err := os.WriteFile(voFile, []byte("domain: from-file\n"), 0o644)
	require.NoError(t, err, "error writing initial values file")

	test.Data.InitialValues = []string{"values-object.yaml"}
	// CLI --set should win over file
	test.Data.Set["domain"] = "from-cli"

	err = test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	assert.Equal(t, "from-cli", out["domain"], "CLI --set should take precedence over initial values file")
}

func TestInitialValuesAbsolutePathResolvesFromRepoRoot(t *testing.T) {
	t.Setenv("ARGOCD_APP_NAME", "test-release")
	t.Setenv("ARGOCD_APP_NAMESPACE", "test")

	test := setupTest(t, testFiles)

	// Place initial values in a subdirectory to simulate repo-root-relative resolution
	ivDir := filepath.Join(test.Settings.Path, "env", "prod")
	require.NoError(t, os.MkdirAll(ivDir, 0o755))

	err := os.WriteFile(
		filepath.Join(ivDir, "initial.yaml"),
		[]byte("foo-bar:\n  logLevel: debug\n"), 0o644,
	)
	require.NoError(t, err)

	// Absolute path resolved from repo root via ArgoCD parameters
	testFilesAsJSON, _ := json.Marshal(testFiles)
	ivFilesAsJSON, _ := json.Marshal([]string{"/env/prod/initial.yaml"})

	t.Setenv(
		"ARGOCD_APP_PARAMETERS",
		`[
			{
				"name":"initialValues",
				"array": `+string(ivFilesAsJSON)+`
			},
			{
				"name":"valueFiles",
				"array": `+string(testFilesAsJSON)+`
			}
		]`,
	)

	err = test.RenderChart(context.Background())
	require.NoError(t, err, "RenderChart should not return an error")
}

// On-the-fly tests: file B (tpl) references values defined in file A (base).
// File order is [tpl, base] so that in reverse traversal base is processed first
// and its resolved values feed into the tpl file's accumulator.
var testFilesOnTheFly = []string{"values.onthefly.tpl.yaml", "values.onthefly.base.yaml"}

func TestOnTheFlyResolvesAcrossFiles(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesOnTheFly)
	test.Settings.OnTheFly = true
	test.Settings.IgnoreEmpty = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	app, ok := out["app"].(map[string]any)
	require.True(t, ok, "Expected 'app' key in output")

	assert.Equal(t, "staging", app["environment"],
		"On-the-fly should resolve cross-file .Values.environment")
	assert.Equal(t, "https://api.eu-west-1.example.com", app["endpoint"],
		"On-the-fly should resolve cross-file .Values.region in template expression")
}

func TestOnTheFlyDisabledLeavesUnresolved(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesOnTheFly)
	test.Settings.OnTheFly = false
	test.Settings.IgnoreEmpty = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error without on-the-fly")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err)

	app, ok := out["app"].(map[string]any)
	require.True(t, ok, "Expected 'app' key in output")

	assert.NotEqual(t, "staging", app["environment"],
		"Without on-the-fly, cross-file values should not be resolved")
}

func TestOnTheFlyInitialValuesWin(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFilesOnTheFly)
	test.Settings.OnTheFly = true
	test.Settings.IgnoreEmpty = true

	// --set should override the base file value
	test.Data.Set["environment"] = "production"

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err)

	app, ok := out["app"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "production", app["environment"],
		"CLI --set should win over file values in on-the-fly mode")
}

func TestOnTheFlyNilValuesNotLeakIntoAccumulator(t *testing.T) {
	t.Parallel()

	// File order: [tpl-referencing-base, nil-file] — reverse processes nil-file first.
	// nil-file has nullified: ~ which should NOT leak into the tpl file's accumulator.
	test := setupTest(t, []string{
		"values.onthefly.tpl.yaml",
		"values.onthefly.nil.yaml",
	})
	test.Settings.OnTheFly = true
	test.Settings.IgnoreEmpty = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues should not return an error")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err)

	// environment and region should resolve from nil.yaml
	app, ok := out["app"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "staging", app["environment"])
	assert.Equal(t, "https://api.eu-west-1.example.com", app["endpoint"])

	// nullified key should be nil in final output (from nil.yaml), not stripped by ResolvedValues
	assert.Nil(t, out["nullified"], "Nil values from files should survive to final merge")
}

func TestOnTheFlyMissingKeyDoesNotPollute(t *testing.T) {
	t.Parallel()

	// missing.tpl.yaml references .Values.doesNotExist — this produces <no value> string.
	// Verify it doesn't pollute the accumulator for other files.
	test := setupTest(t, []string{
		"values.onthefly.base.yaml",
		"values.onthefly.missing.tpl.yaml",
	})
	test.Settings.OnTheFly = true
	test.Settings.IgnoreEmpty = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err)

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err)

	// base.yaml should still resolve cleanly
	assert.Equal(t, "staging", out["environment"])
	assert.Equal(t, "eu-west-1", out["region"])

	// resolved key from missing.tpl should be present
	resolved, ok := out["resolved"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "present", resolved["value"])
}

func TestOnTheFlyWithStripNullValues(t *testing.T) {
	t.Parallel()

	test := setupTest(t, []string{
		"values.onthefly.tpl.yaml",
		"values.onthefly.nil.yaml",
	})
	test.Settings.OnTheFly = true
	test.Settings.IgnoreEmpty = true
	test.Settings.StripNullValues = true

	err := test.TemplateChart(context.Background())
	require.NoError(t, err, "TemplateChart with on-the-fly + StripNullValues should not error")
}

func TestOnTheFlyExistingTestsStillPass(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)
	test.Settings.OnTheFly = true

	err := test.TemplateValues(context.Background())
	require.NoError(t, err, "TemplateValues with on-the-fly should not break existing files")

	out, err := utils.ParseYAMLFile(test.Settings.Out)
	require.NoError(t, err, "The output values should be a valid YAML")

	expected, err := utils.ParseYAMLFile(filepath.Join(test.Settings.Path, "values.expected.yaml"))
	require.NoError(t, err, "Failed to parse expected values file")

	for key, value := range test.Data.Set {
		expected[key] = value
	}

	assert.Equal(t, expected, out,
		"On-the-fly mode should produce identical output for non-cross-referencing files")
}

func TestIncorrectOutputFallback(t *testing.T) {
	t.Parallel()

	test := setupTest(t, testFiles)

	test.Settings.Out = "/non/existing/folder"
	assert.False(t, test.UsingFolderAsOutput(), "UsingFolderAsOutput should return false for a non-existing folder")
	assert.Empty(t, test.Settings.Out, "The output setting should be reset to empty string when the folder does not exist")
}
