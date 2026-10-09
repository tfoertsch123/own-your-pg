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
	"github.com/tfoertsch123/own-your-pg/capture"
	"github.com/tfoertsch123/own-your-pg/help"
)

func main() {
	_, basename := filepath.Split(os.Args[0])

	var args capture.Cli

	l := help.NewHelper()
	argsParser := kong.Must(
		&args,
		kong.Name(basename),
		kong.Vars{
			"basename": basename,
		},
		kong.Help(l.Printer),
	)

	l.CheckTerminal(argsParser.Stdout)
	argsParser.Model.Help = l.Fmt(capture.IntroHelp, capture.SlotHelp)
	
	if _, err := argsParser.Parse(os.Args[1:]); err != nil {
		argsParser.Fatalf("%v", err)
	}

	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
