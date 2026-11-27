package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReader(t *testing.T) {
	in := ": keep-alive\r\n\r\nevent: a\r\ndata: one\r\ndata: two\r\n\r\ndata:{\"x\":1}\n\nevent: tail\ndata: last"
	r := NewReader(strings.NewReader(in))
	want := []Event{{Name: "a", Data: "one\ntwo"}, {Data: `{"x":1}`}, {Name: "tail", Data: "last"}}
	for i, w := range want {
		got, err := r.Next()
		if err != nil || got != w {
			t.Fatalf("event %d = %+v, %v; want %+v", i, got, err, w)
		}
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}
