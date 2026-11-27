package ui

import (
	"sort"
	"strings"
)

// command is a slash command shown in completion and /help.
type command struct {
	Name      string
	Desc      string
	Milestone string // non-empty: not implemented yet, lands in this milestone
}

var commands = []command{
	{Name: "agent", Desc: "Coding agent mode (tools on)"},
	{Name: "chat", Desc: "Plain chat mode (tools off)"},
	{Name: "clear", Desc: "Clear the conversation"},
	{Name: "compact", Desc: "Summarize history to free context", Milestone: "M4"},
	{Name: "cost", Desc: "Show spend and cache-hit rate", Milestone: "M4"},
	{Name: "endpoints", Desc: "Add, edit or delete custom API endpoints"},
	{Name: "help", Desc: "Show commands and shortcuts"},
	{Name: "model", Desc: "Switch model (/model <id> to set directly)"},
	{Name: "official", Desc: "Open the official CLI launcher (claude, codex, agy)", Milestone: "M5"},
	{Name: "provider", Desc: "Switch provider · /provider key | api | url | remove-key"},
	{Name: "quit", Desc: "Exit"},
	{Name: "settings", Desc: "Open settings (F2)"},
	{Name: "theme", Desc: "Pick a color theme (live preview)"},
	{Name: "resume", Desc: "Resume a previous session", Milestone: "M4"},
}

// matchCommands returns commands whose name starts with prefix (without "/").
func matchCommands(prefix string) []command {
	prefix = strings.ToLower(prefix)
	var out []command
	for _, c := range commands {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// parseSlash splits "/name args" into name and args. ok is false for input
// that is not a slash command.
func parseSlash(s string) (name, args string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "", "", false
	}
	s = s[1:]
	name, args, _ = strings.Cut(s, " ")
	return strings.ToLower(name), strings.TrimSpace(args), name != ""
}

// completionQuery returns the partial command name while the user is typing
// the first word of a slash command.
func completionQuery(input string) (string, bool) {
	if !strings.HasPrefix(input, "/") || strings.ContainsAny(input, " \n") {
		return "", false
	}
	return input[1:], true
}
