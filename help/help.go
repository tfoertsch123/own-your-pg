package help

import (
	"bytes"
	"fmt"
	"os"
	"io"
	"errors"

	"github.com/alecthomas/kong"
)

type link struct {
	url string
}

type Helper struct {
	supportedByTerminal bool
	links []link
}

type Stater interface{
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

func NewHelper() *Helper {
	return &Helper{}
}

func (l *Helper) CheckTerminal(f interface{}) {
	l.supportedByTerminal = supportsHyperlinks(f)
}

const linkmarker = "\x00"

// hyperlink wraps text in OSC 8 escape sequences if the terminal
// supports them, otherwise returns the text as-is.
// return fmt.Sprintf("\U0001F517\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
func (l *Helper) hyperlink(url, text string) string {
	if !l.supportedByTerminal {
		return fmt.Sprintf("\U0001F517%s (%s)", text, url)
		return url
	}
	l.links = append(l.links, link{url: url})
	return fmt.Sprintf("%s%s%s", linkmarker, text, linkmarker)
}

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
		
		buf.Write([]byte("\U0001F517\x1b]8;;"))
		buf.Write([]byte(l.links[0].url))
		buf.Write([]byte("\x1b\\"))
		buf.Write(s[:idx])		// linktext
		buf.Write([]byte("\x1b]8;;\x1b\\"))
		
		s = s[idx+len(linkmarker):] // skip 2nd marker
		idx = bytes.Index(s, []byte(linkmarker)) // position to the next link
	}
	buf.Write(s)
}

// HelpF is the signature of help-text functions that take a link
// renderer and return formatted help text.
type HelpF func(func(url, text string) string) string

// delim is a unique marker embedded in the description text to
// separate the intro section from the extra help section. It is
// stripped from the final output by Printer.
const delim = "\n\n:<\x00\x00>:\n\n"

// Fmt renders a HelpF function by passing the hyperlink renderer.
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

		_, err := buf.WriteTo(ctx.Stdout)
		return err
	}

	buf.Write(rest[idx+len(delim):]) // flags
	buf.WriteString("\n")
	l.replaceLinks(rest[:idx], &buf)	// and the after flags part
	buf.WriteString("\n")

	_, err := buf.WriteTo(ctx.Stdout)
	return err
}

// Local Variables:
// tab-width: 4
// End:
