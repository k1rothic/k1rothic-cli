package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestFormatTokens(t *testing.T) {
	cases := map[int]string{
		0: "0", 950: "950", 1000: "1K", 9500: "9.5K", 9999: "10K",
		12_345: "12K", 500_000: "500K", 1_000_000: "1M", 1_250_000: "1.2M",
	}
	for n, want := range cases {
		if got := FormatTokens(n); got != want {
			t.Errorf("FormatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatTokensFine(t *testing.T) {
	cases := map[int]string{312: "312", 1000: "1k", 9450: "9.45k", 9400: "9.4k", 120_500: "120k"}
	for n, want := range cases {
		if got := FormatTokensFine(n); got != want {
			t.Errorf("FormatTokensFine(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	cases := map[time.Duration]string{
		400 * time.Millisecond: "0.4s",
		2 * time.Second:        "2.0s",
		15 * time.Second:       "15s",
		125 * time.Second:      "2m05s",
	}
	for d, want := range cases {
		if got := FormatElapsed(d); got != want {
			t.Errorf("FormatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"home", "me")
	p := filepath.Join(home, "code", "project")
	if got, want := ShortenPath(p, home, 0), filepath.Join("~", "code", "project"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := ShortenPath(home, home, 0); got != "~" {
		t.Fatalf("got %q", got)
	}
	long := filepath.Join(home, "a", "very", "deep", "directory", "structure", "leaf")
	got := ShortenPath(long, home, 20)
	if ansi.StringWidth(got) > 20 {
		t.Fatalf("too wide: %q", got)
	}
	if filepath.Base(got) != "leaf" {
		t.Fatalf("lost leaf: %q", got)
	}
	// Sibling of home must not be rewritten as ~.
	other := filepath.Join(string(filepath.Separator)+"home", "meow")
	if got := ShortenPath(other, home, 0); got != other {
		t.Fatalf("got %q", got)
	}
}
