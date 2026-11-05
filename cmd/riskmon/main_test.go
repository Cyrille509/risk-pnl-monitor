package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEndToEnd(t *testing.T) {
	root := filepath.Join("..", "..")
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{
		"-config", filepath.Join(root, "config.json"),
		"-events", filepath.Join(root, "testdata", "events_day.jsonl"),
		"-audit", auditPath,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	for _, fragment := range []string{
		"WARN event=evt-002", "BREACH event=evt-010", "CLEAR event=evt-012",
		"AAPL qty=-7.000", "total_pnl=-$55.10", "processed_events=13", "replay_throughput=",
	} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Errorf("terminal report missing %q:\n%s", fragment, stdout.String())
		}
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if records := bytes.Count(audit, []byte{'\n'}); records != 21 {
		t.Fatalf("audit records = %d, want 21", records)
	}
}

func TestRunRejectsAuditOverInput(t *testing.T) {
	eventsPath := filepath.Join("..", "..", "testdata", "events_day.jsonl")
	err := run([]string{"-events", eventsPath, "-audit", eventsPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "audit path must differ") {
		t.Fatalf("error = %v, want audit path rejection", err)
	}
}

func TestRunHelp(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"-help"}, &bytes.Buffer{}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(stderr.String(), "-events") || !strings.Contains(stderr.String(), "-audit") {
		t.Fatalf("help output is incomplete:\n%s", stderr.String())
	}
}

func TestSamePathRecognizesExistingHardLink(t *testing.T) {
	directory := t.TempDir()
	original := filepath.Join(directory, "original")
	alias := filepath.Join(directory, "alias")
	if err := os.WriteFile(original, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Link(original, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if !samePath(original, alias) {
		t.Fatal("hard-linked files were not recognized as the same path")
	}
}
