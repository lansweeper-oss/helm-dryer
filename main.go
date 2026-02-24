// Project: dryer
package main

import (
	"github.com/alecthomas/kong"
	cmd "github.com/lansweeper-oss/helm-dryer/cmd/dryer"
)

func main() {
	cli := &cmd.CLI{}
	ctx := kong.Parse(
		cli,
		kong.Description("An ArgoCD CMP to pre-template values files."),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
