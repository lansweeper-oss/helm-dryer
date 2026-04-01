// Package utils provides utility functions for the application.
package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"go.yaml.in/yaml/v3"
)

const (
	ReadOnly     = 0o440
	ReadWrite    = 0o644
	ReadWriteDir = 0o750
)

// templateFuncs is the cached Sprig function map with env/expandenv removed.
// Computed once to avoid re-allocating the large map (~100+ entries) on every template call.
//
//nolint:gochecknoglobals
var templateFuncs template.FuncMap

//nolint:gochecknoinits
func init() {
	templateFuncs = sprig.FuncMap()
	delete(templateFuncs, "env")
	delete(templateFuncs, "expandenv")
	templateFuncs["toYaml"] = toYAML
	templateFuncs["fromYaml"] = fromYAML
}

// fromYAML converts a YAML string into a map[string]any.
func fromYAML(str string) map[string]any {
	result := map[string]any{}

	err := yaml.Unmarshal([]byte(str), &result)
	if err != nil {
		result["Error"] = err.Error()
	}

	return result
}

// toYAML marshals a value to YAML and returns it as a string.
// Unlike Helm's implementation which silently returns "", this returns an error
// so that text/template propagates marshaling failures through Execute.
func toYAML(val any) (string, error) {
	data, err := yaml.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to YAML: %w", err)
	}

	return strings.TrimSuffix(string(data), "\n"), nil
}

// CopyFile copies a file from the source path to the destination path.
func CopyFile(src, dst string) error {
	src = filepath.Clean(src)

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", src, err)
	}

	err = os.WriteFile(dst, data, ReadWrite) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to write destination file %s: %w", dst, err)
	}

	return nil
}

// DeepCopy creates a deep copy of the provided map.
func DeepCopy(src map[string]any) (map[string]any, error) {
	if src == nil {
		return src, nil
	}

	dst := make(map[string]any, len(src))

	for key, value := range src {
		copied, err := innerDeepCopy(value)
		if err != nil {
			return nil, fmt.Errorf("error copying key %s: %w", key, err)
		}

		dst[key] = copied
	}

	return dst, nil
}

// innerDeepCopy recursively copies maps and []any slices.
// Scalars are returned as-is.
// Typed slices (e.g. []string, []int) are not deep-copied since YAML unmarshaling
// only produces []any, and typed slices contain immutable values that are safe to share.
func innerDeepCopy(value any) (any, error) {
	switch val := value.(type) {
	case map[string]any:
		return DeepCopy(val)
	case []any:
		copied := make([]any, len(val))

		for idx, elem := range val {
			c, err := innerDeepCopy(elem)
			if err != nil {
				return nil, err
			}

			copied[idx] = c
		}

		return copied, nil
	default:
		return value, nil
	}
}

// GetEnv retrieves the value of an environment variable.
// If the variable is not set, it returns the provided fallback value.
func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return fallback
}

// GetTemplate returns a new template with Sprig functions, excluding env and expandenv.
// This is to prevent information leakage (injected tokens/passwords).
// Both Helm and ArgoCD remove these due to security implications.
// See: https://masterminds.github.io/sprig/os.html
func GetTemplate(options, left, right string) *template.Template {
	tpl := template.New("manifests")
	tpl.Option(options)
	tpl.Delims(left, right)
	tpl.Funcs(templateFuncs)

	return tpl
}

// GetTTL parses a TTL (Time-to-Live) string in time.Duration format and returns the cutoff time.
// That is, when we say that TTL is 10m, we return the time corresponding to now - 10 minutes.
// Files modified before this cutoff are considered expired.
// If the TTL string is invalid, return a zero time indicating that caching is disabled.
func GetTTL(ttl string) time.Time {
	if ttl == "" {
		return time.Time{}
	}

	duration, err := time.ParseDuration(ttl)
	if err != nil {
		slog.Warn("Invalid TTL format, caching disabled", "ttl", ttl, "error", err)

		return time.Time{}
	}

	return time.Now().Add(-duration)
}

// IsDir checks if the provided path is a directory.
func IsDir(path string) (bool, error) {
	if path == "" {
		return false, nil // empty path falls back to stdout
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("error checking if path is a directory: %w", err)
	}

	return info.IsDir(), nil
}

// ParseYAMLFile reads a YAML file and unmarshals it into a map[string]any map.
// It returns an error if the file cannot be read or if the YAML cannot be unmarshaled.
func ParseYAMLFile(file string) (map[string]any, error) {
	content, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return nil, fmt.Errorf("error reading YAML file %s: %w", file, err)
	}

	return ParseYAML(content)
}

// ParseYAML reads a YAML string and unmarshals it into a map[string]any map.
// An empty or whitespace-only input returns an empty map.
func ParseYAML(content []byte) (map[string]any, error) {
	yamlFile := bytes.NewReader(content)
	decoder := yaml.NewDecoder(yamlFile)
	decoded := make(map[string]any)

	err := decoder.Decode(&decoded)
	if errors.Is(err, io.EOF) {
		return decoded, nil
	}

	if err != nil {
		return nil, fmt.Errorf("error parsing YAML file: %w", err)
	}

	return decoded, nil
}

// ToBoolean converts a string to a boolean value.
func ToBoolean(s string) bool {
	return strings.EqualFold(s, "true")
}

// WriteOutput writes the provided data to a file or stdout.
func WriteOutput(data []byte, out string) error {
	if out == "" || out == "-" || out == "/dev/stdout" {
		_, err := os.Stdout.Write(data)
		if err != nil {
			return fmt.Errorf("error writing output to stdout: %w", err)
		}
	} else {
		err := os.WriteFile(out, data, ReadOnly)
		if err != nil {
			return fmt.Errorf("error writing output to file %s: %w", out, err)
		}
	}

	return nil
}
