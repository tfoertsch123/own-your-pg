package capture

import (
	"strings"
	"testing"
)

func TestIntroHelp(t *testing.T) {
	out := (&Cli{}).IntroHelp(func(url, _ string) string { return url })

	if !strings.Contains(out, "${basename}") {
		t.Error("IntroHelp should contain ${basename} placeholder")
	}
	if !strings.Contains(out, "history file") {
		t.Error("IntroHelp should describe history files")
	}
	if !strings.Contains(out, "incomplete/current") {
		t.Error("IntroHelp should mention incomplete/current")
	}
	if strings.Contains(out, "XXXXXXXX-YYYYYYYY..xxxxxxxx-yyyyyyyy") == false {
		t.Error("IntroHelp should show the file name pattern")
	}
}

func TestExtraHelp(t *testing.T) {
	out := (&Cli{}).ExtraHelp(func(_, text string) string { return text })

	for _, param := range []string{
		"primary_conninfo",
		"primary_slotname",
		"logfile",
		"size_limit",
		"synchronous",
		"ignore_missing_identity",
	} {
		if !strings.Contains(out, param) {
			t.Errorf("ExtraHelp should document %s", param)
		}
	}
	if !strings.Contains(out, "the log package documentation") {
		t.Error("ExtraHelp should contain the link text for log.ParseURL")
	}
	if !strings.Contains(out, "slottool") {
		t.Error("ExtraHelp should mention slottool")
	}
}

func TestExtraHelp_LinksUseRenderer(t *testing.T) {
	var urls []string
	out := (&Cli{}).ExtraHelp(func(url, _ string) string {
		urls = append(urls, url)
		return url
	})

	if len(urls) != 2 {
		t.Fatalf("expected 2 links, got %d", len(urls))
	}
	if !strings.Contains(urls[0], "log#ParseURL") {
		t.Errorf("first link should be log#ParseURL, got %q", urls[0])
	}
	if !strings.Contains(urls[1], "units#ParseStrictBytes") {
		t.Errorf("second link should be units#ParseStrictBytes, got %q", urls[1])
	}
	// With the URL renderer, the URLs must appear in the output.
	if !strings.Contains(out, urls[0]) || !strings.Contains(out, urls[1]) {
		t.Error("rendered URLs should appear in the output")
	}
}

// Local Variables:
// tab-width: 4
// End:
