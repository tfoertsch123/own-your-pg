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
	"github.com/alecthomas/kong"
	"github.com/tfoertsch123/own-your-pg/slottool"
)

func main() {
	var args slottool.Cli
	kong.Parse(&args,
		kong.Name("slottool"),
		kong.Description("Example application demonstrating kong command-line parsing."),
	)

	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
