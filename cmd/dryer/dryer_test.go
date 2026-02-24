package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestMain_Help(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cmd := exec.CommandContext(ctx, "go", "run", "../..", "--help")
	cmd.Env = os.Environ()

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	require.NoError(t, err, "Running the command with --help should not return an error")

	assert.Contains(
		t, out.String(),
		"An ArgoCD CMP to pre-template values files.", "Help output should contain the description",
	)
}

// TestCommand_Help tests the help output of the "render" command.
func TestCommand_Help(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cmd := exec.CommandContext(ctx, "go", "run", "../..", "render", "--help")

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	require.NoError(t, err, "Running the command with --help should not return an error")

	assert.Contains(
		t, out.String(),
		"Render the template as a Configuration Management plugin.", "Help output should contain the command",
	)
}

func TestRunCommand(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const accountId = "234796234"

	var expectedOutputYaml map[string]any

	var outputYaml map[string]any

	tempFile, err := os.CreateTemp(t.TempDir(), "test-run*.yaml")
	require.NoError(t, err, "Creating a temporary file should not return an error")

	testData := "../../internal/dryer/testdata"
	cmd := exec.CommandContext(
		ctx,
		"go", "run", "../..",
		"get",
		"-p", testData,
		"-o", tempFile.Name(),
		"-f", "values.yaml",
		"-f", "values.tpl.yaml",
		"-f", "values.stg.tpl.yaml",
		"-f", "values.does-not-exist.tpl.yaml",
		"-r", "test-template-chart",
		"--set", "accountId="+accountId,
		"--set", "clusterName=eks-cluster-platform",
		"--set", "namePrefixWithoutDomain=eks-cluster",
		"--set", "partition=aws",
		"--ignore-missing",
		"--logging.debug",
	)
	err = cmd.Run()

	require.NoError(t, err, "Running a command should not return an error")
	output, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err, "Reading the output should not return an error")

	err = yaml.Unmarshal(output, &outputYaml)
	require.NoError(t, err, "Decoding the output should not return an error")

	expectedOutput, err := os.ReadFile(filepath.Join(testData, "values.expected.yaml"))
	require.NoError(t, err, "Reading the expected output should not return an error")

	err = yaml.Unmarshal(expectedOutput, &expectedOutputYaml)
	require.NoError(t, err, "Decoding the expected output should not return an error")

	require.Subset(
		t, outputYaml, expectedOutputYaml,
		"Output YAML should be a superset of the expected YAML",
	)

	require.Equal(
		t, accountId, outputYaml["accountId"],
		"The accountId in the output should match the one set in the command",
	)
}
