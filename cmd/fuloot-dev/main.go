// fuloot-dev manages build toolchains independently of the embedded service CLI.
package main

import (
	"fmt"
	"os"

	"github.com/laputacn/fuloot-cli/internal/devtool"
)

func main() {
	if err := devtool.NewCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
