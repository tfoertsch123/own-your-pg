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
	"github.com/tfoertsch123/own-your-pg/m2"
)

func main() {
	_, basename := filepath.Split(os.Args[0])

	var args m2.Cli
	kong.Parse(&args,
		kong.Name(basename),
		kong.Description("Capture PG changes, part of OYPG"),
		kong.Vars{
			"basename": basename,
		},
	)

	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
