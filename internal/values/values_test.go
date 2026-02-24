package values_test

import (
	"testing"

	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	"github.com/lansweeper-oss/helm-dryer/internal/values"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeYAMLArrayOfMaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []map[string]any
		expected map[string]any
		hasError bool
	}{
		{
			name: "Merge two maps with no conflicts",
			input: []map[string]any{
				{"key1": "value1"},
				{"key2": "value2"},
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			hasError: false,
		},
		{
			name: "Merge two maps with conflicts (override)",
			input: []map[string]any{
				{"key1": "value1"},
				{"key1": "value2"},
			},
			expected: map[string]any{
				"key1": "value2",
			},
			hasError: false,
		},
		{
			name: "Merge empty maps",
			input: []map[string]any{
				{},
				{},
			},
			expected: map[string]any{},
			hasError: false,
		},
		{
			name: "Merge single map",
			input: []map[string]any{
				{"key1": "value1"},
			},
			expected: map[string]any{
				"key1": "value1",
			},
			hasError: false,
		},
		{
			name:     "Merge no maps",
			input:    []map[string]any{},
			expected: map[string]any{},
			hasError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, _ := values.MergeYAMLArrayOfMaps(test.input)

			if !test.hasError {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestReadValuesFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		expected map[string]any
		hasError bool
	}{
		{
			name:     "Valid YAML file",
			filePath: "testdata/valid.yaml",
			expected: map[string]any{
				"foo": "bar",
				"baz": "qux",
			},
			hasError: false,
		},
		{
			name:     "Valid YAML file using YAML anchors",
			filePath: "testdata/anchors.yaml",
			expected: map[string]any{
				"foo": map[string]any{
					"a": "a",
					"b": "b",
				},
				"bar": map[string]any{
					"a": "a",
					"b": "b",
					"c": []any{"b"},
				},
			},
			hasError: false,
		},
		{
			name:     "Empty YAML file",
			filePath: "testdata/empty.yaml",
			expected: nil,
			hasError: false,
		},
		{
			name:     "Invalid YAML file",
			filePath: "testdata/invalid.yaml",
			expected: nil,
			hasError: true,
		},
		{
			name:     "Non-existent file",
			filePath: "testdata/nonexistent.yaml",
			expected: nil,
			hasError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, _ := values.ReadFile(test.filePath)

			if !test.hasError {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestMergeYamlMaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		m1       map[string]any
		m2       map[string]any
		expected map[string]any
		hasError bool
	}{
		{
			name: "Merge two maps with no conflicts",
			m1:   map[string]any{"key1": "value1"},
			m2:   map[string]any{"key2": "value2"},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			hasError: false,
		},
		{
			name: "Merge two maps with conflicts (override)",
			m1:   map[string]any{"key1": "value1"},
			m2:   map[string]any{"key1": "value2"},
			expected: map[string]any{
				"key1": "value2",
			},
			hasError: false,
		},
		{
			name: "Merge map with nested structure",
			m1: map[string]any{
				"key1": map[string]any{
					"nestedKey1": "nestedValue1",
				},
			},
			m2: map[string]any{
				"key1": map[string]any{
					"nestedKey2": "nestedValue2",
				},
			},
			expected: map[string]any{
				"key1": map[string]any{
					"nestedKey1": "nestedValue1",
					"nestedKey2": "nestedValue2",
				},
			},
			hasError: false,
		},
		{
			name: "Merge empty map into non-empty map",
			m1:   map[string]any{"key1": "value1"},
			m2:   map[string]any{},
			expected: map[string]any{
				"key1": "value1",
			},
			hasError: false,
		},
		{
			name: "Merge non-empty map into empty map",
			m1:   map[string]any{},
			m2:   map[string]any{"key1": "value1"},
			expected: map[string]any{
				"key1": "value1",
			},
			hasError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_ = values.MergeYamlMaps(test.m1, test.m2)

			if !test.hasError {
				assert.Equal(t, test.expected, test.m1)
			}
		})
	}
}

func TestFromCli(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]any
		hasError bool
	}{
		{
			name: "Simple key-value pairs",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			hasError: false,
		},
		{
			name: "Single nested key",
			input: map[string]string{
				"parent.child": "value1",
			},
			expected: map[string]any{
				"parent": map[string]any{
					"child": "value1",
				},
			},
			hasError: false,
		},
		{
			name: "Multiple nested keys under same parent",
			input: map[string]string{
				"parent.child1": "value1",
				"parent.child2": "value2",
			},
			expected: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child2": "value2",
				},
			},
			hasError: false,
		},
		{
			name: "Deeply nested keys",
			input: map[string]string{
				"level1.level2.level3": "value1",
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "value1",
					},
				},
			},
			hasError: false,
		},
		{
			name: "Mixed simple and nested keys",
			input: map[string]string{
				"simple":        "value1",
				"nested.child":  "value2",
				"simple2":       "value3",
				"nested.child2": "value4",
			},
			expected: map[string]any{
				"simple":  "value1",
				"simple2": "value3",
				"nested": map[string]any{
					"child":  "value2",
					"child2": "value4",
				},
			},
			hasError: false,
		},
		{
			name: "Complex nested structure",
			input: map[string]string{
				"app.database.host":    "localhost",
				"app.database.port":    "5432",
				"app.cache.redis.host": "redis-host",
				"app.cache.redis.port": "6379",
				"app.name":             "myapp",
				"logging.level":        "info",
			},
			expected: map[string]any{
				"app": map[string]any{
					"database": map[string]any{
						"host": "localhost",
						"port": "5432",
					},
					"cache": map[string]any{
						"redis": map[string]any{
							"host": "redis-host",
							"port": "6379",
						},
					},
					"name": "myapp",
				},
				"logging": map[string]any{
					"level": "info",
				},
			},
			hasError: false,
		},
		{
			name:     "Empty input",
			input:    map[string]string{},
			expected: map[string]any{},
			hasError: false,
		},
		{
			name: "Keys with multiple dots",
			input: map[string]string{
				"a.b.c.d": "value1",
				"a.b.e":   "value2",
			},
			expected: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"d": "value1",
						},
						"e": "value2",
					},
				},
			},
			hasError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, _ := values.DotNotationToMap(test.input)

			if !test.hasError {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestDotNotationToMapTypeConflict(t *testing.T) {
	t.Parallel()

	// When a flat key and a nested key share the same prefix, the flat value
	// is not a map, so merging the nested key should return ErrUnexpectedType.
	// Because Go map iteration order is non-deterministic, the error only occurs
	// when the scalar key is processed before the dotted key. We retry to cover
	// both orderings.
	input := map[string]string{
		"key":       "scalar",
		"key.child": "nested",
	}

	var gotError bool

	for range 100 {
		_, err := values.DotNotationToMap(input)
		if err != nil {
			require.ErrorIs(t, err, dryerr.ErrUnexpectedType)

			gotError = true

			break
		}
	}

	require.True(t, gotError, "expected ErrUnexpectedType when flat and nested keys conflict")
}
