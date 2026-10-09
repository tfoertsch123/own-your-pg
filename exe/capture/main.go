// Usage:
//
//	capture <directory> [-S slot]
//
// <directory> is the working directory
package main

import (
	"github.com/tfoertsch123/own-your-pg/help"
	"github.com/tfoertsch123/own-your-pg/capture"
)

func main() {
	var args capture.Cli
	help.ParseArgs(&args, capture.IntroHelp, capture.SlotHelp)
	args.Run()
}

// Local Variables:
// tab-width: 4
// End:
