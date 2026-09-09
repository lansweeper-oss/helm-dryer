package cli

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
)

// ExistingFilesMapper is a kong mapper that splits a comma-separated string into file paths
// and validates that each file exists.
var ExistingFilesMapper = kong.NamedMapper("existingfiles", kong.MapperFunc(decodeExistingFiles))

func decodeExistingFiles(ctx *kong.DecodeContext, target reflect.Value) error {
	var raw string

	err := ctx.Scan.PopValueInto("file", &raw)
	if err != nil {
		return fmt.Errorf("expected comma-separated file paths: %w", err)
	}

	var files []string

	for _, entry := range strings.Split(raw, ",") {
		file := strings.TrimSpace(entry)
		if file == "" {
			continue
		}

		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("initial values file %s: %w", file, err)
		}

		files = append(files, file)
	}

	target.Set(reflect.ValueOf(files))

	return nil
}
