// Package sse is a minimal Server-Sent Events reader for streaming LLM APIs.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// Event is one dispatched SSE event.
type Event struct {
	Name string // "event:" field; empty when absent
	Data string // joined "data:" lines
}

// maxLine bounds a single SSE line (large tool inputs can be long).
const maxLine = 8 << 20

// Reader parses an SSE stream.
type Reader struct {
	sc *bufio.Scanner
}

// NewReader wraps r.
func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	return &Reader{sc: sc}
}

// Next returns the next event, or io.EOF at the end of the stream.
func (r *Reader) Next() (Event, error) {
	var ev Event
	var data []string
	have := false
	for r.sc.Scan() {
		line := strings.TrimSuffix(r.sc.Text(), "\r")
		if line == "" {
			if have {
				ev.Data = strings.Join(data, "\n")
				return ev, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / keep-alive
		}
		field, val, _ := strings.Cut(line, ":")
		val = strings.TrimPrefix(val, " ")
		switch field {
		case "event":
			ev.Name = val
			have = true
		case "data":
			data = append(data, val)
			have = true
		}
	}
	if err := r.sc.Err(); err != nil {
		return Event{}, err
	}
	if have {
		ev.Data = strings.Join(data, "\n")
		return ev, nil
	}
	return Event{}, io.EOF
}
