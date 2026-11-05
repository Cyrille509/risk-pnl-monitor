package riskmon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDeterministicReplay(t *testing.T) {
	config := loadProjectConfig(t)
	events := readProjectFile(t, "testdata", "events_day.jsonl")

	var firstAudit bytes.Buffer
	first, err := RunReplay(bytes.NewReader(events), &firstAudit, config)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	var secondAudit bytes.Buffer
	second, err := RunReplay(bytes.NewReader(events), &secondAudit, config)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}

	if !reflect.DeepEqual(first.Summary, second.Summary) {
		t.Fatalf("summaries differ\nfirst:  %+v\nsecond: %+v", first.Summary, second.Summary)
	}
	if !bytes.Equal(firstAudit.Bytes(), secondAudit.Bytes()) {
		t.Fatalf("audit output is not deterministic\nfirst:\n%s\nsecond:\n%s", firstAudit.Bytes(), secondAudit.Bytes())
	}

	encodedSummary, err := json.MarshalIndent(first.Summary, "", "  ")
	if err != nil {
		t.Fatalf("encode summary: %v", err)
	}
	encodedSummary = append(encodedSummary, '\n')
	golden := readProjectFile(t, "testdata", "golden_summary.json")
	if !bytes.Equal(encodedSummary, golden) {
		t.Fatalf("summary differs from golden\n got:\n%s\nwant:\n%s", encodedSummary, golden)
	}

	lines := nonemptyLines(firstAudit.String())
	wantRecords := int(first.Summary.ProcessedEvents) + first.Summary.AlertsEmitted
	if len(lines) != wantRecords {
		t.Fatalf("audit has %d records, want %d", len(lines), wantRecords)
	}
	if got := auditRecordType(t, lines[0]); got != "event" {
		t.Fatalf("first record type = %q, want event", got)
	}
	if got := auditRecordType(t, lines[1]); got != "event" {
		t.Fatalf("second record type = %q, want event", got)
	}
	if got := auditRecordType(t, lines[2]); got != "alert" {
		t.Fatalf("transition record type = %q, want alert", got)
	}
}

func TestReplayAuditWritesEventBeforeSimultaneousAlerts(t *testing.T) {
	config := Config{
		Bounds: Bounds{MaxPrice: 1_000_000, MaxAbsQuantity: 1_000_000},
		Limits: LimitsConfig{
			MaxAbsPosition:   QtyLimitConfig{Warn: 1_000, Breach: 2_000},
			MaxGrossNotional: MoneyLimitConfig{Warn: 100, Breach: 200},
		},
	}
	events := strings.Join([]string{
		`{"id":"m1","type":"mark","symbol":"ABC","price":"1.00"}`,
		`{"id":"f1","type":"fill","symbol":"ABC","quantity":"2.000","price":"1.00"}`,
	}, "\n")
	var audit bytes.Buffer
	result, err := RunReplay(strings.NewReader(events), &audit, config)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if result.Summary.AlertsEmitted != 2 {
		t.Fatalf("alerts = %d, want 2", result.Summary.AlertsEmitted)
	}
	lines := nonemptyLines(audit.String())
	wantTypes := []string{"event", "event", "alert", "alert"}
	if len(lines) != len(wantTypes) {
		t.Fatalf("audit records = %d, want %d: %s", len(lines), len(wantTypes), audit.String())
	}
	for index, want := range wantTypes {
		if got := auditRecordType(t, lines[index]); got != want {
			t.Fatalf("record %d type = %q, want %q", index, got, want)
		}
	}
	if !strings.Contains(lines[2], `"rule_id":"max_abs_position"`) ||
		!strings.Contains(lines[3], `"rule_id":"max_gross_notional"`) {
		t.Fatalf("unexpected alert order: %s", audit.String())
	}
}

func TestReplayUnmarkedPositionHasZeroGross(t *testing.T) {
	config := loadProjectConfig(t)
	events := `{"id":"f1","type":"fill","symbol":"ABC","quantity":"1.000","price":"10.00"}`
	result, err := RunReplay(strings.NewReader(events), io.Discard, config)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	position := result.Summary.Positions[0]
	if position.LastMark != nil || position.Notional != 0 || result.Summary.GrossNotional != 0 {
		t.Fatalf("unmarked position contributed to gross: %+v summary=%+v", position, result.Summary)
	}
}

func TestEventDecoderReportsLineAndRejectsBlankLine(t *testing.T) {
	config := loadProjectConfig(t)
	decoder := NewEventDecoder(strings.NewReader("\n"), config.Bounds)
	_, err := decoder.Next()
	if err == nil || !strings.Contains(err.Error(), "line 1") || !strings.Contains(err.Error(), "blank") {
		t.Fatalf("blank-line error = %v", err)
	}

	decoder = NewEventDecoder(strings.NewReader(`{"id":"bad","type":"fill","symbol":"ABC","quantity":"1.0000","price":"1.00"}`), config.Bounds)
	_, err = decoder.Next()
	if err == nil || !strings.Contains(err.Error(), "line 1") || !strings.Contains(err.Error(), "at most 3 decimal places") {
		t.Fatalf("precision error = %v", err)
	}
}

func TestEventDecoderReportsOversizedFirstLineAsLineOne(t *testing.T) {
	config := loadProjectConfig(t)
	decoder := NewEventDecoder(strings.NewReader(strings.Repeat("x", maxEventLineBytes+1)), config.Bounds)
	_, err := decoder.Next()
	if err == nil || !strings.Contains(err.Error(), "line 1") || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("oversized-line error = %v", err)
	}
}

func TestReplayPropagatesAuditWriteFailure(t *testing.T) {
	config := loadProjectConfig(t)
	events := `{"id":"f1","type":"fill","symbol":"ABC","quantity":"1.000","price":"10.00"}`
	_, err := RunReplay(strings.NewReader(events), alwaysFailWriter{}, config)
	if err == nil || !strings.Contains(err.Error(), "encode event audit record") {
		t.Fatalf("audit failure = %v", err)
	}
}

func TestSummaryPositionsAreSorted(t *testing.T) {
	config := loadProjectConfig(t)
	events := strings.Join([]string{
		`{"id":"f1","type":"fill","symbol":"ZZZ","quantity":"1.000","price":"1.00"}`,
		`{"id":"f2","type":"fill","symbol":"AAA","quantity":"1.000","price":"1.00"}`,
	}, "\n")
	result, err := RunReplay(strings.NewReader(events), io.Discard, config)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if got := []string{result.Summary.Positions[0].Symbol, result.Summary.Positions[1].Symbol}; !reflect.DeepEqual(got, []string{"AAA", "ZZZ"}) {
		t.Fatalf("position order = %v", got)
	}
}

func TestReplayAggregatesPnLAndGrossAcrossSymbols(t *testing.T) {
	config := Config{
		Bounds: Bounds{MaxPrice: 1_000_000, MaxAbsQuantity: 1_000_000},
		Limits: LimitsConfig{
			MaxAbsPosition:   QtyLimitConfig{Warn: 100_000, Breach: 200_000},
			MaxGrossNotional: MoneyLimitConfig{Warn: 30_000, Breach: 50_000},
		},
	}
	events := strings.Join([]string{
		`{"id":"a-open","type":"fill","symbol":"AAA","quantity":"2.000","price":"100.00"}`,
		`{"id":"a-mark","type":"mark","symbol":"AAA","price":"110.00"}`,
		`{"id":"z-open","type":"fill","symbol":"ZZZ","quantity":"-3.000","price":"50.00"}`,
		`{"id":"z-mark","type":"mark","symbol":"ZZZ","price":"40.00"}`,
		`{"id":"a-reduce","type":"fill","symbol":"AAA","quantity":"-1.000","price":"120.00"}`,
	}, "\n")
	result, err := RunReplay(strings.NewReader(events), io.Discard, config)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if result.Summary.GrossNotional != 23_000 || result.Summary.TotalRealizedPnL != 2_000 ||
		result.Summary.TotalUnrealizedPnL != 4_000 || result.Summary.TotalPnL != 6_000 {
		t.Fatalf("unexpected aggregates: %+v", result.Summary)
	}
	if len(result.Alerts) != 2 || result.Alerts[0].RuleID != RuleMaxGrossNotional ||
		result.Alerts[0].Kind != AlertWarn || result.Alerts[1].Kind != AlertClear {
		t.Fatalf("unexpected aggregate-limit alerts: %+v", result.Alerts)
	}
	if result.Alerts[0].CausingEvent != "z-mark" || result.Alerts[1].CausingEvent != "a-reduce" {
		t.Fatalf("unexpected causing events: %+v", result.Alerts)
	}
}

func TestWriteReportContainsAlertsSummaryAndThroughput(t *testing.T) {
	config := loadProjectConfig(t)
	events := readProjectFile(t, "testdata", "events_day.jsonl")
	result, err := RunReplay(bytes.NewReader(events), io.Discard, config)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	var output bytes.Buffer
	if err := WriteReport(&output, result); err != nil {
		t.Fatalf("write report: %v", err)
	}
	for _, fragment := range []string{
		"WARN event=evt-002", "Final positions", "AAPL qty=-7.000",
		"total_pnl=-$55.10", "gross_notional=$700.00", "replay_throughput=",
	} {
		if !strings.Contains(output.String(), fragment) {
			t.Errorf("report missing %q:\n%s", fragment, output.String())
		}
	}
}

func TestWriteReportDoesNotClaimZeroThroughputBelowTimerResolution(t *testing.T) {
	result := ReplayResult{Summary: Summary{ProcessedEvents: 1}}
	var output bytes.Buffer
	if err := WriteReport(&output, result); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if !strings.Contains(output.String(), "replay_throughput=unavailable") {
		t.Fatalf("missing timer-resolution fallback:\n%s", output.String())
	}
}

func loadProjectConfig(t *testing.T) Config {
	t.Helper()
	data := readProjectFile(t, "config.json")
	config, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode project config: %v", err)
	}
	return config
}

func readProjectFile(t *testing.T, pathParts ...string) []byte {
	t.Helper()
	parts := append([]string{".."}, pathParts...)
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	return data
}

func nonemptyLines(value string) []string {
	return strings.Split(strings.TrimSuffix(value, "\n"), "\n")
}

func auditRecordType(t *testing.T, line string) string {
	t.Helper()
	var record struct {
		RecordType string `json:"record_type"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode audit record: %v", err)
	}
	return record.RecordType
}

type alwaysFailWriter struct{}

func (alwaysFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
