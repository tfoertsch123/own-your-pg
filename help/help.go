// Package help provides helpers for building rich help output with
// [kong].
//
// A [Helper] wraps a [kong.HelpPrinter] that reorders the help text so
// that an "extra" section can appear after the Arguments and Flags
// sections rather than before them. It also detects whether the terminal
// supports OSC 8 hyperlinks and, if so, renders URLs as clickable links;
// otherwise URLs are shown as plain text.
//
// # Usage pattern
//
// A CLI program constructs a [Helper], passes it as the kong.Help
// printer, sets the application description via [Helper.Fmt], and calls
// [Helper.CheckTerminal] after the parser is created:
//
//	l := help.NewHelper()
//	parser := kong.Must(&args,
//	    kong.Name(basename),
//	    kong.Help(l.Printer),
//	)
//	l.CheckTerminal(parser.Stdout)
//	parser.Model.Help = l.Fmt(introFn, extraFn)
//
// introFn and extraFn are [HelpF] functions — they receive a link
// renderer and return a string. The link renderer is [Helper.hyperlink],
// which either wraps the text in OSC 8 sequences or returns it as-is
// depending on terminal capabilities.
package help

import (
	"bytes"
	"fmt"
	"os"
	"io"
	"errors"

	"path/filepath"
	"github.com/alecthomas/kong"
)

type link struct {
	url string
}

// Helper provides a kong.HelpPrinter that reorders help sections
// and renders hyperlinks. It is not safe for concurrent use.
type Helper struct {
	out io.Writer
	supportedByTerminal bool
	links []link
}

// Stater is implemented by *os.File and *os.Std* (both *os.File).
// It is used by [supportsHyperlinks] to check whether the given file
// is a character device (TTY).
type Stater interface {
	Stat() (os.FileInfo, error)
}

// supportsHyperlinks reports whether the terminal likely supports
// OSC 8 hyperlinks. Detection is best-effort: stdout must be a TTY
// and the terminal emulator must be known to support OSC 8.
func supportsHyperlinks(x interface{}) bool {
	f, ok := x.(Stater)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "vscode", "ghostty":
		return true
	case "Apple_Terminal":
		return true
	}
	if os.Getenv("COLORTERM") == "truecolor" {
		return true
	}
	return false
}

// NewHelper creates a [Helper] with no links and terminal detection
// disabled. Call [Helper.CheckTerminal] after the kong parser is
// constructed to enable hyperlink rendering.
func NewHelper() *Helper {
	return &Helper{}
}

// CheckTerminal detects whether the given writer (typically the
// kong parser's Stdout) is a TTY connected to a terminal that supports
// OSC 8 hyperlinks. This must be called before [Helper.Fmt] so that
// the link renderer knows whether to emit escape sequences.
func (l *Helper) CheckTerminal(f interface{}) {
	l.supportedByTerminal = supportsHyperlinks(f)
}

// TerminalSupportsLinks sets/returns whether the output supports OSC 8
// escape sequences. If called without a parameter, the current state
// is returned. This state is set either by a previous call on
// TerminalSupportsLinks with a parameter or by calling on CheckTerminal.
// If a true parameter is passed, the output is declared to support OSC 8
// escape sequences. A false parameter turns them off. In either case, the
// previous value is returned.
func (l *Helper) TerminalSupportsLinks(set ...bool) bool {
	if len(set) > 0 {
		defer func() {
			l.supportedByTerminal = set[0]
		}()
	}
	return l.supportedByTerminal
}

// linkmarker is a NUL byte used to delimit link text within the
// rendered help string. It is replaced by OSC 8 escape sequences (or
// plain text) in [Helper.replaceLinks].
const linkmarker = "\x00"

// hyperlink renders a URL as an OSC 8 hyperlink if the terminal
// supports it, or as a plain-text fallback. The visible text is
// marked with [linkmarker] delimiters so that [Helper.replaceLinks]
// can substitute the full escape sequence in a post-processing pass.
func (l *Helper) hyperlink(url, text string) string {
	if !l.supportedByTerminal {
		return fmt.Sprintf("\U0001F517%s (%s)", text, url)
	}
	l.links = append(l.links, link{url: url})
	return fmt.Sprintf("%s%s%s", linkmarker, text, linkmarker)
}

// replaceLinks scans s for [linkmarker]-delimited regions and
// writes the full OSC 8 escape sequences (or the link text) into buf.
// The links are consumed in order from [Helper.links].
func (l *Helper) replaceLinks(s []byte, buf io.Writer) {
	// buf is expected to be a bytes.Buffer here. No error handling needed.
	if l.links == nil {
		buf.Write(s)
		return
	}

	for idx := bytes.Index(s, []byte(linkmarker)); idx >= 0; {
		buf.Write(s[:idx])		// before the link
		s = s[idx+len(linkmarker):] // skip marker
		idx = bytes.Index(s, []byte(linkmarker)) // closing marker
		if idx < 0 {
			panic(errors.New("unbalanced link markers"))
		}

		if len(l.links) < 1 {
			panic(errors.New("more link markers than links"))
		}
		
		buf.Write([]byte("\U0001F517\x1b]8;;"))
		buf.Write([]byte(l.links[0].url))
		buf.Write([]byte("\x1b\\"))
		buf.Write(s[:idx])		// linktext
		buf.Write([]byte("\x1b]8;;\x1b\\"))
		l.links = l.links[1:]
		
		s = s[idx+len(linkmarker):] // skip 2nd marker
		idx = bytes.Index(s, []byte(linkmarker)) // position to the next link
	}
	buf.Write(s)
}

// HelpF is the signature of help-text functions that take a link
// renderer and return formatted help text.
// HelpF is the signature of help-text functions. The link
// argument is a renderer (typically [Helper.hyperlink]) that wraps
// a URL and its visible text into a terminal-appropriate hyperlink.
type HelpF func(func(url, text string) string) string

// delim is a delimiter string embedded between help sections by
// [Helper.Fmt]. [Helper.Printer] splits the captured output at these
// delimiters to move the extra section after the Flags.
// delim is a unique marker embedded in the description text to
// separate the intro section from the extra help section. It is
// stripped from the final output by Printer.
const delim = "\n\n:<\x00\x00>:\n\n"

// Fmt renders a HelpF function by passing the hyperlink renderer.
// Fmt renders two help sections and joins them with [delim].
// before is printed before the Arguments/Flags (as the kong
// description), after is moved to the end by [Helper.Printer].
// If after is nil only the before section and a single delimiter
// are returned.
func (l *Helper) Fmt(before, after HelpF) string {
	l.links = nil				// just in case
	if after == nil {
		return before(l.hyperlink) + delim
	}
	return before(l.hyperlink) + delim + after(l.hyperlink) + delim
}

// Printer returns a kong.HelpPrinter that captures the default help
// output, splits it at the delimiter boundaries, and reassembles it
// so that the extra help text appears after the Arguments/Flags
// section instead of before it.
// Printer implements [kong.HelpPrinter]. It captures the default
// help output into a buffer, splits it at [delim] boundaries, and
// reassembles it so that the "after" section appears after the Flags.
// Hyperlink markers are replaced with their final rendering in the
// process.
func (l *Helper) Printer(opts kong.HelpOptions, ctx *kong.Context) error {
	var buf bytes.Buffer
	origStdout := ctx.Stdout
	ctx.Stdout = &buf
	if err := kong.DefaultHelpPrinter(opts, ctx); err != nil {
		ctx.Stdout = origStdout
		return err
	}
	ctx.Stdout = origStdout

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())

	idx := bytes.Index(out, []byte(delim))
	if idx < 0 {			// no delimiter
		// this is only possible if the description did not go through
		// Fmt(). In this case we don't need to handle hyperlinks.
		_, err := ctx.Stdout.Write(out)
		return err
	}

	buf.Reset()
	l.replaceLinks(out[:idx], &buf) // before flags
	buf.WriteString("\n\n")

	rest := out[idx+len(delim):]

	idx = bytes.Index(rest, []byte(delim))
	if idx < 0 {
		// there is no second part. So, finish it up.
		buf.Write(rest)
	} else {
		buf.Write(rest[idx+len(delim):]) // flags
		buf.WriteString("\n")
		l.replaceLinks(rest[:idx], &buf) // and the after flags part
		buf.WriteString("\n")
	}
	_, err := buf.WriteTo(ctx.Stdout)
	return err
}

// ParseArgs implements my standard command line parsing. It is
// equivalent to this code:
//  _, basename := filepath.Split(os.Args[0])
//  
//  l := help.NewHelper()
//  argsParser := kong.Must(
//  	args,
//  	kong.Name(basename),
//  	kong.Vars{
//  		"basename": basename,
//  	},
//  	kong.Help(l.Printer),
//  )
//  
//  l.CheckTerminal(argsParser.Stdout)
//  argsParser.Model.Help = l.Fmt(beforeArgs, afterArgs)
//  
//  if _, err := argsParser.Parse(os.Args[1:]); err != nil {
//  	argsParser.Fatalf("%v", err)
//  }
func ParseArgs(args interface{}, beforeArgs, afterArgs HelpF) {
	_, basename := filepath.Split(os.Args[0])

	l := NewHelper()
	argsParser := kong.Must(
		args,
		kong.Name(basename),
		kong.Vars{
			"basename": basename,
		},
		kong.Help(l.Printer),
	)

	l.CheckTerminal(argsParser.Stdout)
	argsParser.Model.Help = l.Fmt(beforeArgs, afterArgs)
	
	if _, err := argsParser.Parse(os.Args[1:]); err != nil {
		argsParser.Fatalf("%v", err)
	}
}

// Local Variables:
// tab-width: 4
// End:
