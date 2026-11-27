package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// FormatTokens renders a token count compactly: 950, 9.5K, 500K, 1.2M.
func FormatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1000)) + "K"
	case n < 1_000_000:
		return fmt.Sprintf("%dK", (n+500)/1000)
	default:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1_000_000)) + "M"
	}
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

// FormatElapsed renders durations for the status line: 0.4s, 15s, 2m05s.
func FormatElapsed(d time.Duration) string {
	switch {
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// ShortenPath replaces the home dir with ~ and, if still wider than max cells,
// keeps the trailing path components behind an ellipsis.
func ShortenPath(p, home string, maxW int) string {
	if home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				p = "~"
			} else {
				p = "~" + string(filepath.Separator) + rel
			}
		}
	}
	if maxW <= 0 || ansi.StringWidth(p) <= maxW {
		return p
	}
	sep := string(filepath.Separator)
	parts := strings.Split(p, sep)
	// Drop leading components until it fits.
	for i := 1; i < len(parts); i++ {
		cand := "…" + sep + strings.Join(parts[i:], sep)
		if ansi.StringWidth(cand) <= maxW {
			return cand
		}
	}
	return ansi.TruncateLeft(p, ansi.StringWidth(p)-maxW+1, "…")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// FormatTokensFine renders counts for the status line: 312, 9.45k, 120k.
func FormatTokensFine(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 100_000:
		s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(n)/1000), "0"), ".")
		return s + "k"
	default:
		return fmt.Sprintf("%dk", n/1000)
	}
}
