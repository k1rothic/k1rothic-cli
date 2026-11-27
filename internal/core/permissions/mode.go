// Package permissions holds approval modes and (from M3) allow/deny policy.
package permissions

import "fmt"

// Mode is the tool approval mode.
type Mode int

const (
	// Ask prompts before every side-effecting tool call. The default.
	Ask Mode = iota
	// AutoEdit applies file edits without asking but still asks for bash/web.
	AutoEdit
	// Plan is read-only: no edits, writes or shell commands.
	Plan
	// AlwaysApprove runs every tool call without asking. Requires explicit
	// opt-in and is never reached by cycling unless enabled.
	AlwaysApprove
)

var names = map[Mode]string{
	Ask:           "ask",
	AutoEdit:      "auto-edit",
	Plan:          "plan",
	AlwaysApprove: "always-approve",
}

func (m Mode) String() string {
	if s, ok := names[m]; ok {
		return s
	}
	return fmt.Sprintf("mode(%d)", int(m))
}

// Parse converts a config string to a Mode.
func Parse(s string) (Mode, error) {
	for m, n := range names {
		if n == s {
			return m, nil
		}
	}
	return Ask, fmt.Errorf("unknown approval mode %q (want ask, auto-edit, plan or always-approve)", s)
}

// Next returns the mode after m in the Shift+Tab cycle. AlwaysApprove is only
// part of the cycle when allowAlways is true (explicit user opt-in).
func (m Mode) Next(allowAlways bool) Mode {
	cycle := []Mode{Ask, AutoEdit, Plan}
	if allowAlways {
		cycle = append(cycle, AlwaysApprove)
	}
	for i, c := range cycle {
		if c == m {
			return cycle[(i+1)%len(cycle)]
		}
	}
	return Ask
}
