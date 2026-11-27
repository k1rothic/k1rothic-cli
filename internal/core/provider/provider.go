// Package provider defines the single normalized streaming interface every
// LLM backend adapter (Anthropic, OpenAI, Gemini, OpenAI-compatible) implements.
//
// Adapters translate vendor wire formats into a stream of [Event] values so
// that the agent loop and UI never see vendor-specific shapes.
package provider

import (
	"context"
	"encoding/json"
)

// Provider streams one model turn.
//
// Stream must return a channel that is closed after exactly one terminal event
// (EventDone or EventError) has been sent. Cancelling ctx must stop the stream
// promptly; the adapter then sends an EventError wrapping ctx.Err() (or simply
// closes after EventDone if the turn already finished).
type Provider interface {
	// Name is the stable provider identifier, e.g. "anthropic".
	Name() string
	Stream(ctx context.Context, req Request) <-chan Event
}

// Role of a message author.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Effort is the requested reasoning effort. Adapters map it to the vendor's
// closest setting (or ignore it for models without reasoning controls).
type Effort string

const (
	EffortOff    Effort = "off"
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Message is one entry of the append-only conversation history.
type Message struct {
	Role       Role
	Text       string
	ToolCalls  []ToolCall // assistant messages only
	ToolCallID string     // tool-result messages only
}

// ToolSchema describes a tool the model may call.
type ToolSchema struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolCall is a fully assembled tool invocation requested by the model.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Request is one model turn. System and Tools form the stable, cacheable
// prefix; Messages is append-only.
type Request struct {
	Model     string
	System    string
	Tools     []ToolSchema // nil/empty in chat mode
	Messages  []Message
	Effort    Effort
	MaxTokens int
}

// EventKind discriminates [Event].
type EventKind int

const (
	EventTextDelta EventKind = iota + 1
	EventReasoningDelta
	EventToolCall
	EventUsage
	EventError
	EventDone
)

func (k EventKind) String() string {
	switch k {
	case EventTextDelta:
		return "text"
	case EventReasoningDelta:
		return "reasoning"
	case EventToolCall:
		return "tool_call"
	case EventUsage:
		return "usage"
	case EventError:
		return "error"
	case EventDone:
		return "done"
	}
	return "unknown"
}

// Usage reports token counts. Providers may emit several usage events per
// turn (e.g. input at start, output at end); values are cumulative for the
// turn, so consumers should keep the latest.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
}

// Total is the context consumed by this turn (prompt + completion).
func (u Usage) Total() int {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens + u.OutputTokens
}

// StopReason explains why a turn ended.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopCancelled StopReason = "cancelled"
	StopRefusal   StopReason = "refusal"
)

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	ID            string
	Name          string // display name; may be empty
	ContextWindow int    // input tokens; 0 = unknown
}

// ModelLister is implemented by providers that can list their models.
type ModelLister interface {
	Models(ctx context.Context) ([]ModelInfo, error)
}

// Event is one item of a normalized stream. Only the field matching Kind is set.
type Event struct {
	Kind     EventKind
	Text     string // EventTextDelta, EventReasoningDelta
	ToolCall *ToolCall
	Usage    *Usage
	Err      error
	Stop     StopReason // EventDone
}
