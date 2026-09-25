// Command herdr-dstask is a Herdr plugin that lists, shows and changes dstask
// tasks in a terminal UI. It reads and writes the task repository through the
// dstask library, not the dstask binary.
package main

import (
	"os"

	"github.com/cgardner/herdr-dstask/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
