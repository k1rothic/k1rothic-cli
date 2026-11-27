package fake

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
)

func collect(ch <-chan provider.Event) []provider.Event {
	var evs []provider.Event
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func TestStreamOrderAndTerminal(t *testing.T) {
	p := &Provider{Reply: func(provider.Request) (string, string) { return "think", "hello world" }}
	evs := collect(p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Text: "hi"}}}))

	var text, reasoning strings.Builder
	sawText := false
	for _, ev := range evs {
		switch ev.Kind {
		case provider.EventReasoningDelta:
			if sawText {
				t.Fatal("reasoning after text")
			}
			reasoning.WriteString(ev.Text)
		case provider.EventTextDelta:
			sawText = true
			text.WriteString(ev.Text)
		}
	}
	if text.String() != "hello world" || reasoning.String() != "think" {
		t.Fatalf("text=%q reasoning=%q", text.String(), reasoning.String())
	}
	last := evs[len(evs)-1]
	if last.Kind != provider.EventDone || last.Stop != provider.StopEndTurn {
		t.Fatalf("last event = %+v", last)
	}
}

func TestStreamCancel(t *testing.T) {
	p := &Provider{Delay: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	ch := p.Stream(ctx, provider.Request{})
	<-ch // first usage event
	cancel()

	done := make(chan []provider.Event)
	go func() { done <- collect(ch) }()
	select {
	case evs := <-done:
		for _, ev := range evs {
			if ev.Kind == provider.EventDone {
				t.Fatal("got done after cancel")
			}
		}
		if n := len(evs); n > 0 && evs[n-1].Kind == provider.EventError && !errors.Is(evs[n-1].Err, context.Canceled) {
			t.Fatalf("unexpected error %v", evs[n-1].Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after cancel")
	}
}

func TestChunksUTF8(t *testing.T) {
	got := chunks("héllo wörld ✓", 3)
	if strings.Join(got, "") != "héllo wörld ✓" {
		t.Fatalf("lossy chunking: %q", got)
	}
}
