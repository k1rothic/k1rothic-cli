// Package fake is a scripted [provider.Provider] used by the M1 UI shell and
// by tests. It streams reasoning, then markdown text in small chunks, then a
// usage event, honouring context cancellation at every step.
package fake

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
)

// Provider streams canned responses.
type Provider struct {
	// Delay between chunks. Zero means no delay (useful in tests).
	Delay time.Duration
	// ChunkRunes is the size of each text delta in runes (default 6).
	ChunkRunes int
	// Reply, if set, overrides the generated response for every turn.
	Reply func(req provider.Request) (reasoning, text string)
}

// New returns a provider tuned for an interactive demo.
func New() *Provider {
	return &Provider{Delay: 18 * time.Millisecond, ChunkRunes: 5}
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return "fake" }

// Stream implements provider.Provider.
func (p *Provider) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		reasoning, text := p.reply(req)

		send := func(ev provider.Event) bool {
			select {
			case <-ctx.Done():
				return false
			case out <- ev:
				return true
			}
		}
		sleep := func() bool {
			if p.Delay <= 0 {
				return ctx.Err() == nil
			}
			t := time.NewTimer(p.Delay)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-t.C:
				return true
			}
		}
		cancelled := func() {
			// Best effort: the consumer may have stopped reading already.
			select {
			case out <- provider.Event{Kind: provider.EventError, Err: ctx.Err()}:
			case <-time.After(100 * time.Millisecond):
			}
		}

		input := estimateTokens(req)
		if !send(provider.Event{Kind: provider.EventUsage, Usage: &provider.Usage{InputTokens: input}}) {
			cancelled()
			return
		}
		for _, chunk := range chunks(reasoning, p.chunkSize()) {
			if !sleep() || !send(provider.Event{Kind: provider.EventReasoningDelta, Text: chunk}) {
				cancelled()
				return
			}
		}
		outTok := 0
		for _, chunk := range chunks(text, p.chunkSize()) {
			if !sleep() || !send(provider.Event{Kind: provider.EventTextDelta, Text: chunk}) {
				cancelled()
				return
			}
			outTok += approxTokens(chunk)
			if outTok%20 == 0 {
				send(provider.Event{Kind: provider.EventUsage, Usage: &provider.Usage{InputTokens: input, OutputTokens: outTok}})
			}
		}
		outTok = approxTokens(reasoning) + approxTokens(text)
		if !send(provider.Event{Kind: provider.EventUsage, Usage: &provider.Usage{
			InputTokens: input, OutputTokens: outTok, ReasoningTokens: approxTokens(reasoning),
		}}) {
			cancelled()
			return
		}
		send(provider.Event{Kind: provider.EventDone, Stop: provider.StopEndTurn})
	}()
	return out
}

func (p *Provider) chunkSize() int {
	if p.ChunkRunes <= 0 {
		return 6
	}
	return p.ChunkRunes
}

func (p *Provider) reply(req provider.Request) (string, string) {
	if p.Reply != nil {
		return p.Reply(req)
	}
	last := ""
	if n := len(req.Messages); n > 0 {
		last = req.Messages[n-1].Text
	}
	reasoning := "The user said something; this is the fake provider, so I will " +
		"produce a canned markdown sample that exercises the renderer."
	text := "## Fake response\n\n" +
		"You said: *" + oneLine(last) + "*\n\n" +
		"This is **M1**: the UI shell streams from a scripted provider so the\n" +
		"layout can be tested without API keys.\n\n" +
		"- Headings, **bold**, `inline code`\n" +
		"- Lists and horizontal rules\n" +
		"- Highlighted code blocks\n\n" +
		"---\n\n" +
		"```go\nfunc main() {\n\tfmt.Println(\"hello from k1rothic\")\n}\n```\n\n" +
		"1. First item\n2. Second item\n"
	return reasoning, text
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 60 {
		r := []rune(s)
		s = string(r[:59]) + "…"
	}
	return s
}

// chunks splits s into pieces of n runes.
func chunks(s string, n int) []string {
	var out []string
	r := []rune(s)
	for len(r) > 0 {
		k := min(n, len(r))
		out = append(out, string(r[:k]))
		r = r[k:]
	}
	return out
}

func approxTokens(s string) int {
	if s == "" {
		return 0
	}
	return max(1, utf8.RuneCountInString(s)/4)
}

func estimateTokens(req provider.Request) int {
	n := approxTokens(req.System)
	for _, m := range req.Messages {
		n += approxTokens(m.Text) + 4
	}
	for _, t := range req.Tools {
		n += approxTokens(t.Description) + len(t.InputSchema)/4
	}
	return n
}
