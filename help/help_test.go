package help

import (
	"bytes"
	"strings"
	"os"
	"time"
	"testing"

	"github.com/alecthomas/kong"
)

// noLink is a link renderer that returns the URL as-is (no OSC 8).
func noLink(url, text string) string {
	return url
}

// fakeStater implements Stater for testing supportsHyperlinks.
type fakeStater struct {
	isCharDevice bool
}

func (f fakeStater) Stat() (os.FileInfo, error) {
	return fakeFileInfo{isCharDevice: f.isCharDevice}, nil
}

type fakeFileInfo struct {
	isCharDevice bool
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode {
	if f.isCharDevice {
		return os.ModeCharDevice
	}
	return 0
}
func (f fakeFileInfo) ModTime() time.Time  { return time.Time{} }
func (f fakeFileInfo) IsDir() bool         { return false }
func (f fakeFileInfo) Sys() interface{}    { return nil }

// ---- supportsHyperlinks ----

func TestSupportsHyperlinks_NonStater(t *testing.T) {
	if supportsHyperlinks("not a stater") {
		t.Error("expected false for non-Stater")
	}
}

func TestSupportsHyperlinks_NonCharDevice(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if supportsHyperlinks(fakeStater{isCharDevice: false}) {
		t.Error("expected false for non-char-device")
	}
}

func TestSupportsHyperlinks_CharDeviceKnownTerm(t *testing.T) {
	for _, term := range []string{"iTerm.app", "WezTerm", "vscode", "ghostty", "Apple_Terminal"} {
		t.Run(term, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", term)
			if !supportsHyperlinks(fakeStater{isCharDevice: true}) {
				t.Errorf("expected true for TERM_PROGRAM=%s", term)
			}
		})
	}
}

func TestSupportsHyperlinks_CharDeviceTruecolor(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("COLORTERM", "truecolor")
	if !supportsHyperlinks(fakeStater{isCharDevice: true}) {
		t.Error("expected true for COLORTERM=truecolor")
	}
}

func TestSupportsHyperlinks_CharDeviceUnknownTerm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "xterm")
	t.Setenv("COLORTERM", "")
	if supportsHyperlinks(fakeStater{isCharDevice: true}) {
		t.Error("expected false for unknown terminal")
	}
}

// ---- Helper.CheckTerminal / TerminalSupportsLinks ----

func TestCheckTerminal_NonStater(t *testing.T) {
	l := NewHelper()
	l.CheckTerminal("not a stater")
	if l.TerminalSupportsLinks() {
		t.Error("expected false after CheckTerminal with non-Stater")
	}
}

func TestCheckTerminal_CharDeviceKnownTerm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	l := NewHelper()
	l.CheckTerminal(fakeStater{isCharDevice: true})
	if !l.TerminalSupportsLinks() {
		t.Error("expected true after CheckTerminal with iTerm.app")
	}
}

func TestTerminalSupportsLinks_SetGet(t *testing.T) {
	l := NewHelper()
	if l.TerminalSupportsLinks() {
		t.Error("expected false initially")
	}
	previous := l.TerminalSupportsLinks(true)
	if previous {
		t.Error("expected previous value to be false")
	}
	if !l.TerminalSupportsLinks() {
		t.Error("expected true after setting")
	}
	previous = l.TerminalSupportsLinks(false)
	if !previous {
		t.Error("expected previous value to be true")
	}
	if l.TerminalSupportsLinks() {
		t.Error("expected false after clearing")
	}
}

// ---- hyperlink ----

func TestHyperlink_NoTerminalSupport(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(false)
	out := l.hyperlink("https://example.com", "example")
	// When terminal doesn't support links, returns a 🔗-prefixed fallback
	if !strings.Contains(out, "example") {
		t.Errorf("expected visible text, got: %q", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected URL in fallback, got: %q", out)
	}
	// No link should be recorded
	if len(l.links) != 0 {
		t.Errorf("expected 0 links, got %d", len(l.links))
	}
}

func TestHyperlink_WithTerminalSupport(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)
	out := l.hyperlink("https://example.com", "example")
	// Output should contain the linkmarker-delimited text
	if !strings.Contains(out, linkmarker+"example"+linkmarker) {
		t.Errorf("expected marker-delimited text, got: %q", out)
	}
	// One link should be recorded
	if len(l.links) != 1 || l.links[0].url != "https://example.com" {
		t.Errorf("expected 1 link to example.com, got %v", l.links)
	}
}

// ---- replaceLinks ----

func TestReplaceLinks_NoLinks(t *testing.T) {
	l := NewHelper()
	var buf bytes.Buffer
	input := []byte("plain text with no links")
	l.replaceLinks(input, &buf)
	if buf.String() != "plain text with no links" {
		t.Errorf("got %q, want %q", buf.String(), "plain text with no links")
	}
}

func TestReplaceLinks_WithLinks(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)
	// Simulate two hyperlinks being created
	l.hyperlink("https://a.com", "linkA")
	l.hyperlink("https://b.com", "linkB")

	input := []byte("before\x00linkA\x00mid\x00linkB\x00after")
	var buf bytes.Buffer
	l.replaceLinks(input, &buf)

	out := buf.String()
	// First link should have OSC 8 with a.com
	if !strings.Contains(out, "\x1b]8;;https://a.com\x1b\\linkA\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 for a.com, got: %q", out)
	}
	// Second link should have OSC 8 with b.com
	if !strings.Contains(out, "\x1b]8;;https://b.com\x1b\\linkB\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 for b.com, got: %q", out)
	}
	// Non-link text should be preserved
	if !strings.Contains(out, "before") || !strings.Contains(out, "mid") ||
		!strings.Contains(out, "after") {
		t.Errorf("expected surrounding text preserved, got: %q", out)
	}
}

func TestReplaceLinks_UnbalancedMarkers(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)
	l.hyperlink("https://a.com", "linkA")

	input := []byte("before\x00linkA") // no closing marker
	var buf bytes.Buffer

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unbalanced markers")
		}
	}()
	l.replaceLinks(input, &buf)
}

// ---- Fmt ----

func TestFmt_BothSections(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)

	before := func(link func(url, text string) string) string {
		return "intro text"
	}
	after := func(link func(url, text string) string) string {
		return "extra text"
	}

	out := l.Fmt(before, after)

	// Should contain both sections separated by delim
	if !strings.HasPrefix(out, "intro text") {
		t.Errorf("expected intro first, got: %q", out)
	}
	if !strings.Contains(out, delim) {
		t.Errorf("expected delimiter, got: %q", out)
	}
	if !strings.Contains(out, "extra text") {
		t.Errorf("expected extra text, got: %q", out)
	}
	// Should end with delim
	if !strings.HasSuffix(out, delim) {
		t.Errorf("expected trailing delim, got: %q", out)
	}
}

func TestFmt_NilAfter(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)

	before := func(link func(url, text string) string) string {
		return "intro only"
	}

	out := l.Fmt(before, nil)

	if !strings.HasPrefix(out, "intro only") {
		t.Errorf("expected intro only, got: %q", out)
	}
	if !strings.HasSuffix(out, delim) {
		t.Errorf("expected trailing delim, got: %q", out)
	}
	// Should contain exactly one delim
	if strings.Count(out, delim) != 1 {
		t.Errorf("expected 1 delimiter, got %d", strings.Count(out, delim))
	}
}

func TestFmt_ResetLinks(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)

	// Add some stale links
	l.links = append(l.links, link{url: "stale"})

	before := func(link func(url, text string) string) string {
		return "intro"
	}
	after := func(link func(url, text string) string) string {
		return link("https://fresh.com", "fresh") + " text"
	}

	l.Fmt(before, after)

	// Fmt should have reset links before processing
	// Only the link from `after` should be recorded
	if len(l.links) != 1 || l.links[0].url != "https://fresh.com" {
		t.Errorf("expected 1 fresh link, got %v", l.links)
	}
}

// ---- Printer ----

// testCLI is a minimal kong struct for Printer tests.
type testCLI struct {
	Dir string `arg:"" required:"" help:"Working directory."`
}

func newTestKong(t *testing.T, desc string, printer kong.HelpPrinter) (*kong.Kong, *bytes.Buffer) {
	t.Helper()
	var args testCLI
	var out bytes.Buffer
	k := kong.Must(&args,
		kong.Name("testapp"),
		kong.Writers(&out, &out),
		kong.Help(printer),
		kong.ConfigureHelp(kong.HelpOptions{NoAppDescFormat: true}),
	)
	k.Model.Help = desc
	// Prevent os.Exit during --help in tests
	k.Exit = func(int) {}
	return k, &out
}

func TestPrinter_WithDelimiters(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(false) // plain text, no OSC 8

	desc := "intro text" + delim + "extra text" + delim
	k, out := newTestKong(t, desc, l.Printer)

	_, _ = k.Parse([]string{".", "--help"})

	result := out.String()

	// The intro should appear before Arguments
	introIdx := strings.Index(result, "intro text")
	argsIdx := strings.Index(result, "Arguments:")
	if introIdx < 0 {
		t.Fatal("intro text not found")
	}
	if argsIdx < 0 {
		t.Fatal("Arguments: not found")
	}
	if introIdx > argsIdx {
		t.Error("intro text should appear before Arguments")
	}

	// The extra text should appear after Flags
	extraIdx := strings.Index(result, "extra text")
	flagsIdx := strings.Index(result, "Flags:")
	if extraIdx < 0 {
		t.Fatal("extra text not found")
	}
	if flagsIdx < 0 {
		t.Fatal("Flags: not found")
	}
	if extraIdx < flagsIdx {
		t.Error("extra text should appear after Flags")
	}

	// No delimiter should appear in output
	if strings.Contains(result, delim) {
		t.Error("delimiter should be stripped from output")
	}
}

func TestPrinter_NoDelimiter(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(false)

	desc := "just a plain description"
	k, out := newTestKong(t, desc, l.Printer)

	_, _ = k.Parse([]string{".", "--help"})

	result := out.String()

	// Should contain the description as-is
	if !strings.Contains(result, "just a plain description") {
		t.Errorf("description not found in output: %q", result)
	}
	// Should contain Arguments and Flags
	if !strings.Contains(result, "Arguments:") {
		t.Error("Arguments: not found")
	}
	if !strings.Contains(result, "Flags:") {
		t.Error("Flags: not found")
	}
}

func TestPrinter_WithLinks(t *testing.T) {
	l := NewHelper()
	l.TerminalSupportsLinks(true)

	before := func(link func(url, text string) string) string {
		return "intro"
	}
	after := func(link func(url, text string) string) string {
		return "see " + link("https://example.com", "docs") + " for info"
	}
	desc := l.Fmt(before, after)

	k, out := newTestKong(t, desc, l.Printer)
	_, _ = k.Parse([]string{".", "--help"})

	result := out.String()

	// Should contain OSC 8 hyperlink
	if !strings.Contains(result, "\x1b]8;;https://example.com\x1b\\docs\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 hyperlink in output: %q", result)
	}
	// Should not contain the raw linkmarker
	if strings.Contains(result, linkmarker) {
		t.Errorf("linkmarker should be replaced: %q", result)
	}
	// Extra text should appear after Flags
	extraIdx := strings.Index(result, "for info")
	flagsIdx := strings.Index(result, "Flags:")
	if extraIdx < 0 {
		t.Fatal("extra text not found")
	}
	if flagsIdx < 0 {
		t.Fatal("Flags: not found")
	}
	if extraIdx < flagsIdx {
		t.Error("extra text should appear after Flags")
	}
}

// Local Variables:
// tab-width: 4
// End:
