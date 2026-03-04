// Package values provides functions to operate with YAML values for Helm charts.
package values

import (
	"fmt"
	"strings"

	"dario.cat/mergo"
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
)

// MergeYamlMaps merges two Values maps deeply from left to right.
func MergeYamlMaps(m1 map[string]any, m2 map[string]any) error {
	err := mergo.Merge(&m1, m2, mergo.WithOverride)
	if err != nil {
		return fmt.Errorf("failed to merge maps: %w", err)
	}

	return nil
}

// MergeYAMLArrayOfMaps deeply merges multiple YAML maps into a single map.
func MergeYAMLArrayOfMaps(maps []map[string]any) (map[string]any, error) {
	merged := map[string]any{}
	for _, m := range maps {
		err := MergeYamlMaps(merged, m)
		if err != nil {
			return nil, fmt.Errorf("failed to merge array of maps: %w", err)
		}
	}

	return merged, nil
}

// DotNotationToMap reads a map[string]string coming from CLI input by supporting nested keys using
// dot notation (e.g., "key.subkey").
func DotNotationToMap(m map[string]string) (map[string]any, error) {
	vals := make(map[string]any, len(m))

	for key, val := range m {
		if strings.Contains(key, ".") { //nolint:nestif
			parts := strings.SplitN(key, ".", 2) //nolint:mnd
			// Recursive call for nested keys
			innerValue, err := DotNotationToMap(map[string]string{parts[1]: val})
			if err != nil {
				return nil, fmt.Errorf("failed to convert nested key %s: %w", key, err)
			}

			if vals[parts[0]] == nil {
				vals[parts[0]] = innerValue
			} else {
				existingValue, ok := vals[parts[0]].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w %s, got %T", dryerr.ErrUnexpectedType, parts[0], vals[parts[0]])
				}

				err = MergeYamlMaps(existingValue, innerValue)
				if err != nil {
					return nil, fmt.Errorf("failed to merge nested key %s: %w", key, err)
				}
			}
		} else {
			vals[key] = val
		}
	}

	return vals, nil
}
