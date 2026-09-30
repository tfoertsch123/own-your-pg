// Usage:
//
//	slottool <directory> [-S slot] [-l lsn] [-t type] [-c name[=value]]...
//
// <directory> is the working directory or the slot directory inside a
// working directory.
//
// Each -c option takes either "name=string" or "name=JSON-array" or
// "name={shell-quoted list of words}". If only a name is given, the
// config is deleted from the slot.
// Multiple -c options are accumulated.
package main

import (
	"os"
	"path/filepath"
	"github.com/alecthomas/kong"
	"github.com/tfoertsch123/own-your-pg/slottool"
)

func main() {
	_, basename := filepath.Split(os.Args[0])

	var args slottool.Cli
	kong.Parse(&args,
		kong.Name(basename),
		kong.Description("A tool to create, list and modify OYPG slots"),
		kong.Vars{
			"basename": basename,
		},
	)

	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
