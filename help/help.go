package help

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// supportsHyperlinks reports whether the terminal likely supports
// OSC 8 hyperlinks. Detection is best-effort: stdout must be a TTY
// and the terminal emulator must be known to support OSC 8.
func supportsHyperlinks() bool {
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "vscode", "ghostty":
		return true
	case "Apple_Terminal":
		return true
	}
	// GNOME Terminal, kitty, and others set COLORTERM=truecolor
	// and are generally OSC 8 capable.
	if os.Getenv("COLORTERM") == "truecolor" {
		return true
	}
	return false
}

// hyperlink wraps text in OSC 8 escape sequences if the terminal
// supports them, otherwise returns the text as-is.
 func hyperlink(url, text string) string {
	if !supportsHyperlinks() {
		return url
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

type HelpF func(func(url, text string) string) string
func Printer(hlp HelpF) kong.HelpPrinter {
	return func(opts kong.HelpOptions, ctx *kong.Context) error {
		if err := kong.DefaultHelpPrinter(opts, ctx); err != nil {
			return err
		}
		_, err := ctx.Stdout.Write([]byte("\n" + Fmt(hlp) + "\n"))
		return err
	}
}

func Fmt(hlp HelpF) string {
	return hlp(hyperlink)
}

// Local Variables:
// tab-width: 4
// End:
