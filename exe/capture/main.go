// Usage:
//
//	capture <directory> [-S slot]
//
// <directory> is the working directory
package main

import (
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/tfoertsch123/own-your-pg/help"
	"github.com/tfoertsch123/own-your-pg/capture"
)

func main() {
	_, basename := filepath.Split(os.Args[0])

	var args capture.Cli
	kong.Parse(&args,
		kong.Name(basename),
		kong.Description(help.Fmt(capture.IntroHelp)),
		kong.Vars{
			"basename": basename,
		},
		kong.Help(help.Printer(capture.SlotHelp)),
	)

	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
