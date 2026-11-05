package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	riskmon "riskmon/internal"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "riskmon: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("riskmon", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config.json", "path to the JSON configuration")
	eventsPath := flags.String("events", "testdata/events_day.jsonl", "path to ordered event JSONL")
	auditPath := flags.String("audit", "audit.jsonl", "path for deterministic audit JSONL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if samePath(*auditPath, *eventsPath) || samePath(*auditPath, *configPath) {
		return fmt.Errorf("audit path must differ from config and events paths")
	}

	configFile, err := os.Open(*configPath)
	if err != nil {
		return fmt.Errorf("open config %q: %w", *configPath, err)
	}
	config, decodeErr := riskmon.DecodeConfig(configFile)
	closeConfigErr := configFile.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeConfigErr != nil {
		return fmt.Errorf("close config %q: %w", *configPath, closeConfigErr)
	}

	eventsFile, err := os.Open(*eventsPath)
	if err != nil {
		return fmt.Errorf("open events %q: %w", *eventsPath, err)
	}
	defer eventsFile.Close()
	auditFile, err := os.OpenFile(*auditPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open audit %q: %w", *auditPath, err)
	}
	auditBuffer := bufio.NewWriterSize(auditFile, 64*1024)
	result, replayErr := riskmon.RunReplay(eventsFile, auditBuffer, config)
	flushErr := auditBuffer.Flush()
	closeAuditErr := auditFile.Close()
	if replayErr != nil {
		return replayErr
	}
	if flushErr != nil {
		return fmt.Errorf("flush audit %q: %w", *auditPath, flushErr)
	}
	if closeAuditErr != nil {
		return fmt.Errorf("close audit %q: %w", *auditPath, closeAuditErr)
	}
	if err := riskmon.WriteReport(stdout, result); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func samePath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	if filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute) {
		return true
	}
	leftInfo, leftStatErr := os.Stat(leftAbsolute)
	rightInfo, rightStatErr := os.Stat(rightAbsolute)
	return leftStatErr == nil && rightStatErr == nil && os.SameFile(leftInfo, rightInfo)
}
