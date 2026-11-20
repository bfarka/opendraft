package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "opendraft") {
		t.Fatalf("expected output to contain %q, got %q", "opendraft", buf.String())
	}
}
