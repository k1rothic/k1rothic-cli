package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSplitBlocks(t *testing.T) {
	src := "# Title\n\nPara one\nstill one.\n\n- a\n\n- b\n\n```go\nx := 1\n\ny := 2\n```\n\nTail"
	got := SplitBlocks(src)
	want := []string{
		"# Title",
		"Para one\nstill one.",
		"- a\n\n- b",
		"```go\nx := 1\n\ny := 2\n```",
		"Tail",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d blocks: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitBlocksUnclosedFence(t *testing.T) {
	got := SplitBlocks("intro\n\n```\ncode\n\nmore")
	if len(got) != 2 || got[1] != "```\ncode\n\nmore" {
		t.Fatalf("got %q", got)
	}
}

func TestCloseOpenFence(t *testing.T) {
	if got := closeOpenFence("```go\nx"); got != "```go\nx\n```" {
		t.Fatalf("got %q", got)
	}
	if got := closeOpenFence("```\nx\n```"); got != "```\nx\n```" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderIncrementalMatchesContent(t *testing.T) {
	r := New(true, 0, 60)
	full := "## Heading\n\nSome **bold** text.\n\n```go\nfmt.Println(1)\n```\n"
	var last string
	for i := 1; i <= len(full); i++ {
		last = r.Render(full[:i])
	}
	plain := ansi.Strip(last)
	for _, s := range []string{"Heading", "bold", "fmt.Println(1)"} {
		if !strings.Contains(plain, s) {
			t.Fatalf("rendered output missing %q:\n%s", s, plain)
		}
	}
	if len(r.cache) == 0 {
		t.Fatal("expected completed blocks to be cached")
	}
}

func TestRenderCRLF(t *testing.T) {
	r := New(true, 0, 40)
	if out := ansi.Strip(r.Render("a\r\n\r\nb")); strings.Contains(out, "\r") {
		t.Fatalf("CR leaked into output: %q", out)
	}
}

func TestRenderWidth(t *testing.T) {
	r := New(true, 0, 30)
	out := r.Render(strings.Repeat("word ", 50))
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 30 {
			t.Fatalf("line width %d > 30: %q", w, l)
		}
	}
}
