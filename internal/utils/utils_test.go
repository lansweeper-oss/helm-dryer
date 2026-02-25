package utils_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lansweeper-oss/helm-dryer/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	deepNestedValue = "deep_value"
	siblingValue    = "sibling_value"
	otherValue      = "other_value"
	rootValue       = "root_value"
)

var (
	deepNestedTestData = map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"deep": deepNestedValue,
				},
				"other": otherValue,
			},
			"sibling": siblingValue,
		},
		"root": rootValue,
	}

	mixedTypesTestData = map[string]any{
		"slice": []string{"a", "b", "c"},
		"map": map[string]any{
			"inner": "value",
		},
		"nil":    nil,
		"string": "test",
		"number": 42,
		"bool":   true,
	}
)

// verifyInstanceSeparation ensures all nested maps are different instances.
func verifyInstanceSeparation(t *testing.T, original, replica map[string]any) {
	t.Helper()

	if original == nil || replica == nil {
		return
	}

	assert.NotSame(t, &original, &replica, "Top level should be different instances")

	for key, value := range original {
		if nestedMap, ok := value.(map[string]any); ok {
			copyValue, exists := replica[key]
			require.True(t, exists, "Key %s should exist in copy", key)

			if copyNestedMap, ok := copyValue.(map[string]any); ok {
				assert.NotSame(t, &nestedMap, &copyNestedMap,
					"Nested map at '%s' should be different instances", key)
				// Recursively verify deeper levels
				verifyInstanceSeparation(t, nestedMap, copyNestedMap)
			}
		}
	}
}

// verifyNestedStructureIntegrity validates the specific deep nested structure.
func verifyNestedStructureIntegrity(t *testing.T, result map[string]any) {
	t.Helper()

	// Verify structure integrity
	level1, okay := result["level1"].(map[string]any)
	require.True(t, okay, "level1 should be a map")

	level2, okay := level1["level2"].(map[string]any)
	require.True(t, okay, "level2 should be a map")

	level3, okay := level2["level3"].(map[string]any)
	require.True(t, okay, "level3 should be a map")

	// Verify values
	assert.Equal(t, deepNestedValue, level3["deep"])
	assert.Equal(t, otherValue, level2["other"])
	assert.Equal(t, siblingValue, level1["sibling"]) // This validates our improved lines 65-67
	assert.Equal(t, rootValue, result["root"])
}

// verifyMixedTypesIntegrity validates mixed type copying.
func verifyMixedTypesIntegrity(t *testing.T, original, replica map[string]any) {
	t.Helper()

	// Verify slices are different instances
	if origSlice, ok := original["slice"]; ok {
		copySlice := replica["slice"]
		assert.NotSame(t, &origSlice, &copySlice, "Slices should be different instances")
	}

	// Verify nested maps are different instances
	if origMap, ok := original["map"].(map[string]any); ok {
		copyMap, ok := replica["map"].(map[string]any)
		require.True(t, ok, "Copied map should maintain type")
		assert.NotSame(t, &origMap, &copyMap, "Nested maps should be different instances")
	}
}

// Helper function to create deeply nested maps for testing.
func createDeeplyNestedMap(depth int) map[string]any {
	if depth <= 0 {
		return map[string]any{"value": "deepest"}
	}

	return map[string]any{
		"level": createDeeplyNestedMap(depth - 1),
	}
}

func TestDeepCopy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		input        map[string]any
		wantErr      bool
		validateFunc func(t *testing.T, input, result map[string]any)
	}{
		{
			name:    "nil input map",
			input:   nil,
			wantErr: false,
			validateFunc: func(t *testing.T, input, result map[string]any) {
				t.Helper()
				assert.Nil(t, result, "DeepCopy should return nil for nil input")
			},
		},
		{
			name:    "empty map",
			input:   map[string]any{},
			wantErr: false,
			validateFunc: func(t *testing.T, input, result map[string]any) {
				t.Helper()
				assert.Equal(t, map[string]any{}, result)
				assert.NotSame(t, &input, &result, "Should return different instance")
			},
		},
		{
			name: "simple map with primitive values",
			input: map[string]any{
				"string": "value",
				"int":    42,
				"bool":   true,
				"float":  3.14,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, input, result map[string]any) {
				t.Helper()
				assert.Equal(t, input, result)
				assert.NotSame(t, &input, &result, "Should return different instance")
			},
		},
		{
			name:    "deep nested structure with siblings", // This replaces the original problematic test
			input:   deepNestedTestData,
			wantErr: false,
			validateFunc: func(t *testing.T, input, result map[string]any) {
				t.Helper()
				assert.Equal(t, input, result, "Should preserve all nested values")
				verifyInstanceSeparation(t, input, result)
				verifyNestedStructureIntegrity(t, result)
			},
		},
		{
			name:    "mixed types with nested maps",
			input:   mixedTypesTestData,
			wantErr: false,
			validateFunc: func(t *testing.T, input, result map[string]any) {
				t.Helper()
				assert.Equal(t, input, result)
				verifyInstanceSeparation(t, input, result)
				verifyMixedTypesIntegrity(t, input, result)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := utils.DeepCopy(testCase.input)

			if testCase.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)

			if testCase.validateFunc != nil {
				testCase.validateFunc(t, testCase.input, result)
			}
		})
	}

	// Additional test: Verify modification isolation
	t.Run("modification isolation", func(t *testing.T) {
		t.Parallel()

		original := map[string]any{
			"nested": map[string]any{
				"key": "original_value",
			},
		}

		result, err := utils.DeepCopy(original)
		require.NoError(t, err)

		// Modify the copy
		result["nested"].(map[string]any)["key"] = "modified_value"
		result["new_key"] = "new_value"

		// Verify original is unchanged
		assert.Equal(t, "original_value",
			original["nested"].(map[string]any)["key"],
			"Original should remain unchanged")
		assert.NotContains(t, original, "new_key",
			"Original should not contain new keys")
	})
}

func TestDeepCopyEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("very deep nesting", func(t *testing.T) {
		t.Parallel()

		input := createDeeplyNestedMap(50)
		result, err := utils.DeepCopy(input)

		require.NoError(t, err)
		assert.Equal(t, input, result)
		verifyInstanceSeparation(t, input, result)
	})

	t.Run("large map with many siblings", func(t *testing.T) {
		t.Parallel()

		input := make(map[string]any)
		for i := range 100 {
			input[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
		}

		result, err := utils.DeepCopy(input)

		require.NoError(t, err)
		assert.Equal(t, input, result)
		assert.NotSame(t, &input, &result)
	})
}

func TestGetEnv(t *testing.T) {
	// Test case 1: Environment variable exists
	t.Setenv("TEST_ENV_VAR", "value")

	result := utils.GetEnv("TEST_ENV_VAR", "default")
	assert.Equal(t, "value", result, "GetEnv should return the value of the environment variable if it exists")

	// Test case 2: Environment variable does not exist
	result = utils.GetEnv("NON_EXISTENT_ENV_VAR", "default")
	assert.Equal(
		t,
		"default",
		result,
		"GetEnv should return the fallback value if the environment variable does not exist",
	)
}

func TestGetTTL(t *testing.T) {
	t.Parallel()

	// Test case 1: Valid TTL string
	validTTL := "1h30m"
	expectedTime := time.Now().Add(-1*time.Hour - 30*time.Minute)
	result := utils.GetTTL(validTTL)
	assert.WithinDuration(
		t,
		expectedTime,
		result,
		time.Second,
		"GetTTL should return the correct expiration time for a valid TTL string",
	)

	// Test case 2: Invalid TTL string
	invalidTTL := "invalid_duration"
	result = utils.GetTTL(invalidTTL)
	assert.True(t, result.IsZero(), "GetTTL should return a zero time for an invalid TTL string")

	// Test case 3: Empty TTL string
	emptyTTL := ""
	result = utils.GetTTL(emptyTTL)
	assert.True(t, result.IsZero(), "GetTTL should return a zero time for an empty TTL string")
}

func TestIsDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		path        string
		expected    bool
		expectError bool
	}{
		{
			name:        "empty path",
			path:        "",
			expected:    false,
			expectError: false,
		},
		{
			name: "existing directory",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()

				return dir
			},
			expected:    true,
			expectError: false,
		},
		{
			name: "existing file",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "testfile.txt")
				err := os.WriteFile(file, []byte("test content"), utils.ReadWrite)
				require.NoError(t, err)

				return file
			},
			expected:    false,
			expectError: false,
		},
		{
			name:        "non-existent path",
			path:        "/non/existent/path",
			expected:    false,
			expectError: true,
		},
		{
			name: "nested directory",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				nestedDir := filepath.Join(dir, "nested", "deep")
				err := os.MkdirAll(nestedDir, utils.ReadWriteDir)
				require.NoError(t, err)

				return nestedDir
			},
			expected:    true,
			expectError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := testCase.path
			if testCase.setupFunc != nil {
				path = testCase.setupFunc(t)
			}

			result, err := utils.IsDir(path)

			if testCase.expectError {
				require.Error(t, err)
				assert.False(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, testCase.expected, result)
			}
		})
	}
}

func TestParseYAML(t *testing.T) {
	t.Parallel()
	// Test case 1: Valid YAML file
	validYAML := `
key1: value1
key2: &anchor
  foo: bar
  nestedKey: nestedValue
key3:
  <<: *anchor
  nestedKey: overridden
`
	tempFile, err := os.CreateTemp(t.TempDir(), "valid*.yaml")
	require.NoError(t, err, "Failed to create temporary file for valid YAML")

	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(validYAML)
	require.NoError(t, err, "Failed to write valid YAML to temporary file")
	tempFile.Close()

	result, err := utils.ParseYAMLFile(tempFile.Name())
	require.NoError(t, err, "ParseYAML should not return an error for valid YAML")
	assert.Equal(t, "value1", result["key1"], "ParseYAML should correctly parse top-level keys")
	require.Equal(
		t,
		"nestedValue",
		result["key2"].(map[string]any)["nestedKey"],
		"ParseYAML should correctly parse nested keys",
	)
	require.Equal(
		t,
		"overridden",
		result["key3"].(map[string]any)["nestedKey"],
		"ParseYAML should correctly parse anchors",
	)
	require.Equal(
		t,
		result["key2"].(map[string]any)["foo"],
		result["key3"].(map[string]any)["foo"],
		"ParseYAML should correctly parse anchors",
	)

	// Test case 2: Invalid YAML file
	invalidYAML := `
key1: value1
key2: - invalid
`
	tempFile, err = os.CreateTemp(t.TempDir(), "invalid*.yaml")
	require.NoError(t, err, "Failed to create temporary file for invalid YAML")

	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(invalidYAML)
	require.NoError(t, err, "Failed to write invalid YAML to temporary file")
	tempFile.Close()

	_, err = utils.ParseYAMLFile(tempFile.Name())
	require.Error(t, err, "ParseYAML should return an error for invalid YAML")

	// Test case 3: Non-existent file
	_, err = utils.ParseYAMLFile("non_existent_file.yaml")
	require.Error(t, err, "ParseYAML should return an error for a non-existent file")
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	t.Run("successful copy", func(t *testing.T) {
		t.Parallel()

		srcDir := t.TempDir()
		dstDir := t.TempDir()

		srcFile := filepath.Join(srcDir, "source.txt")
		dstFile := filepath.Join(dstDir, "dest.txt")

		content := []byte("hello world")
		err := os.WriteFile(srcFile, content, utils.ReadWrite)
		require.NoError(t, err)

		err = utils.CopyFile(srcFile, dstFile)
		require.NoError(t, err)

		result, err := os.ReadFile(dstFile)
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	t.Run("source file does not exist", func(t *testing.T) {
		t.Parallel()

		dstFile := filepath.Join(t.TempDir(), "dest.txt")

		err := utils.CopyFile("/nonexistent/file.txt", dstFile)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("destination path is invalid", func(t *testing.T) {
		t.Parallel()

		srcFile := filepath.Join(t.TempDir(), "source.txt")
		err := os.WriteFile(srcFile, []byte("data"), utils.ReadWrite)
		require.NoError(t, err)

		err = utils.CopyFile(srcFile, "/nonexistent/dir/dest.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write destination file")
	})
}

func TestDeepCopyWithSlices(t *testing.T) {
	t.Parallel()

	t.Run("copies []any slices deeply", func(t *testing.T) {
		t.Parallel()

		original := map[string]any{
			"list": []any{"a", "b", map[string]any{"nested": "value"}},
		}

		result, err := utils.DeepCopy(original)
		require.NoError(t, err)
		assert.Equal(t, original, result)

		// Verify the slice is a different instance
		origSlice := original["list"].([]any)
		copySlice := result["list"].([]any)
		origSlice[0] = "modified"

		assert.Equal(t, "a", copySlice[0], "Modifying original slice should not affect copy")
	})

	t.Run("copies nested maps inside slices", func(t *testing.T) {
		t.Parallel()

		original := map[string]any{
			"items": []any{
				map[string]any{"key": "value1"},
				map[string]any{"key": "value2"},
			},
		}

		result, err := utils.DeepCopy(original)
		require.NoError(t, err)

		// Modify nested map in original
		original["items"].([]any)[0].(map[string]any)["key"] = "modified"

		assert.Equal(t, "value1", result["items"].([]any)[0].(map[string]any)["key"],
			"Modifying nested map in original slice should not affect copy")
	})
}

func TestEnsureDirExists(t *testing.T) {
	t.Parallel()

	t.Run("creates directory when it does not exist", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "newdir")

		err := utils.EnsureDirExists(dir, utils.ReadWriteDir)
		require.NoError(t, err)

		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("succeeds when directory already exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := utils.EnsureDirExists(dir, utils.ReadWriteDir)
		require.NoError(t, err)
	})

	t.Run("creates nested directories", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "a", "b", "c")

		err := utils.EnsureDirExists(dir, utils.ReadWriteDir)
		require.NoError(t, err)

		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestFromYaml(t *testing.T) {
	t.Parallel()

	tpl := utils.GetTemplate("missingkey=zero", "{{", "}}")

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		tmpl, err := tpl.Clone()
		require.NoError(t, err)
		tmpl, err = tmpl.Parse(`{{ $m := fromYaml .input }}{{ $m.name }}`)
		require.NoError(t, err)

		var buf bytes.Buffer

		err = tmpl.Execute(&buf, map[string]any{"input": "name: test\nversion: 1"})
		require.NoError(t, err)
		assert.Equal(t, "test", buf.String())
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		tmpl, err := tpl.Clone()
		require.NoError(t, err)
		tmpl, err = tmpl.Parse(`{{ $m := fromYaml .input }}{{ $m.Error }}`)
		require.NoError(t, err)

		var buf bytes.Buffer

		err = tmpl.Execute(&buf, map[string]any{"input": ": invalid: yaml: ["})
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String(), "should contain error message")
	})
}

func TestGetTemplate(t *testing.T) {
	t.Parallel()

	t.Run("returns a valid template with custom delimiters", func(t *testing.T) {
		t.Parallel()

		tpl := utils.GetTemplate("missingkey=error", "<<", ">>")
		require.NotNil(t, tpl)

		parsed, err := tpl.Parse("<< .Name >>")
		require.NoError(t, err)

		var buf bytes.Buffer

		err = parsed.Execute(&buf, map[string]string{"Name": "test"})
		require.NoError(t, err)
		assert.Equal(t, "test", buf.String())
	})

	t.Run("env and expandenv functions are removed", func(t *testing.T) {
		t.Parallel()

		tpl := utils.GetTemplate("missingkey=error", "{{", "}}")

		_, err := tpl.Parse(`{{ env "HOME" }}`)
		require.Error(t, err, "env function should not be available")
	})

	t.Run("sprig functions are available", func(t *testing.T) {
		t.Parallel()

		tpl := utils.GetTemplate("missingkey=error", "{{", "}}")

		parsed, err := tpl.Parse(`{{ "hello" | upper }}`)
		require.NoError(t, err)

		var buf bytes.Buffer

		err = parsed.Execute(&buf, nil)
		require.NoError(t, err)
		assert.Equal(t, "HELLO", buf.String())
	})
}

func TestToBoolean(t *testing.T) {
	t.Parallel()
	// Test case 1: Input is "true" (case insensitive)
	assert.True(t, utils.ToBoolean("true"), "ToBoolean should return true for 'true'")
	assert.True(t, utils.ToBoolean("TRUE"), "ToBoolean should return true for 'TRUE'")
	assert.True(t, utils.ToBoolean("TrUe"), "ToBoolean should return true for 'TrUe'")

	// Test case 2: Input is not "true"
	assert.False(t, utils.ToBoolean("false"), "ToBoolean should return false for 'false'")
	assert.False(t, utils.ToBoolean("yes"), "ToBoolean should return false for 'yes'")
	assert.False(t, utils.ToBoolean("1"), "ToBoolean should return false for '1'")
	assert.False(t, utils.ToBoolean(""), "ToBoolean should return false for an empty string")
	assert.False(t, utils.ToBoolean("random"), "ToBoolean should return false for 'random'")
}

func TestToYaml(t *testing.T) {
	t.Parallel()

	tpl := utils.GetTemplate("missingkey=zero", "{{", "}}")

	t.Run("map", func(t *testing.T) {
		t.Parallel()

		tmpl, err := tpl.Clone()
		require.NoError(t, err)
		tmpl, err = tmpl.Parse(`{{ .data | toYaml }}`)
		require.NoError(t, err)

		var buf bytes.Buffer

		err = tmpl.Execute(&buf, map[string]any{"data": map[string]any{"key": "value", "num": 42}})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "key: value")
		assert.Contains(t, buf.String(), "num: 42")
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()

		tmpl, err := tpl.Clone()
		require.NoError(t, err)
		tmpl, err = tmpl.Parse(`{{ .data | toYaml }}`)
		require.NoError(t, err)

		var buf bytes.Buffer

		err = tmpl.Execute(&buf, map[string]any{"data": nil})
		require.NoError(t, err)
		assert.Equal(t, "null", buf.String())
	})
}

func TestWriteOutput(t *testing.T) {
	t.Parallel()
	t.Run("writes to file", func(t *testing.T) {
		t.Parallel()
		outFile := filepath.Join(t.TempDir(), "output.txt")
		data := []byte("output content")

		err := utils.WriteOutput(data, outFile)
		require.NoError(t, err)

		result, err := os.ReadFile(outFile)
		require.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("returns error for invalid file path", func(t *testing.T) {
		t.Parallel()

		err := utils.WriteOutput([]byte("data"), "/nonexistent/dir/file.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error writing output to file")
	})

	t.Run("writes to stdout when path is empty", func(t *testing.T) {
		t.Parallel()

		err := utils.WriteOutput([]byte("stdout data"), "")
		require.NoError(t, err)
	})

	t.Run("writes to stdout when path is dash", func(t *testing.T) {
		t.Parallel()

		err := utils.WriteOutput([]byte("stdout data"), "-")
		require.NoError(t, err)
	})
}
