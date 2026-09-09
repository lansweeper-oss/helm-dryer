// Project: dryer
package main

import (
	"github.com/alecthomas/kong"
	"github.com/lansweeper-oss/helm-dryer/internal/cli"
	cmd "github.com/lansweeper-oss/helm-dryer/cmd/dryer"
)

func main() {
	c := &cmd.CLI{}
	ctx := kong.Parse(
		c,
		kong.Description("An ArgoCD CMP to pre-template values files."),
		cli.ExistingFilesMapper,
	)
	ctx.FatalIfErrorf(ctx.Run())
}
