package riskmon

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

const maxEventLineBytes = 1 << 20

type EventDecoder struct {
	scanner *bufio.Scanner
	bounds  Bounds
	line    int
}

func NewEventDecoder(reader io.Reader, bounds Bounds) *EventDecoder {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxEventLineBytes)
	return &EventDecoder{scanner: scanner, bounds: bounds}
}

func (decoder *EventDecoder) Next() (Event, error) {
	if !decoder.scanner.Scan() {
		if err := decoder.scanner.Err(); err != nil {
			return Event{}, fmt.Errorf("read events at line %d: %w", decoder.line+1, err)
		}
		return Event{}, io.EOF
	}
	decoder.line++
	line := decoder.scanner.Bytes()
	if len(strings.TrimSpace(string(line))) == 0 {
		return Event{}, fmt.Errorf("line %d: blank lines are not valid events", decoder.line)
	}
	event, err := DecodeEvent(line, decoder.bounds)
	if err != nil {
		return Event{}, fmt.Errorf("line %d: %w", decoder.line, err)
	}
	return event, nil
}

type ReplayResult struct {
	Summary         Summary
	Alerts          []Alert
	Elapsed         time.Duration
	EventsPerSecond float64
}

func RunReplay(events io.Reader, audit io.Writer, config Config) (ReplayResult, error) {
	engine, err := NewEngine(config)
	if err != nil {
		return ReplayResult{}, err
	}
	if audit == nil {
		audit = io.Discard
	}
	decoder := NewEventDecoder(events, config.Bounds)
	auditWriter := NewAuditWriter(audit)
	started := time.Now()
	for {
		event, err := decoder.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ReplayResult{}, err
		}
		result, err := engine.Apply(event)
		if err != nil {
			return ReplayResult{}, err
		}
		if err := auditWriter.WriteResult(result); err != nil {
			return ReplayResult{}, fmt.Errorf("event %q: %w", event.ID, err)
		}
	}
	elapsed := time.Since(started)
	summary, err := engine.Summary()
	if err != nil {
		return ReplayResult{}, err
	}
	throughput := float64(0)
	if elapsed > 0 {
		throughput = float64(summary.ProcessedEvents) / elapsed.Seconds()
	}
	return ReplayResult{
		Summary:         summary,
		Alerts:          engine.Alerts(),
		Elapsed:         elapsed,
		EventsPerSecond: throughput,
	}, nil
}

func WriteReport(writer io.Writer, result ReplayResult) error {
	var report strings.Builder
	report.WriteString("Alerts\n")
	if len(result.Alerts) == 0 {
		report.WriteString("  none\n")
	}
	for _, alert := range result.Alerts {
		scope := ""
		if alert.Symbol != "" {
			scope = " symbol=" + alert.Symbol
		}
		fmt.Fprintf(&report, "  %s event=%s rule=%s%s %s->%s value=%s threshold=%s\n",
			alert.Kind, alert.CausingEvent, alert.RuleID, scope, alert.PreviousState,
			alert.NewState, formatAlertValue(alert.RuleID, alert.MeasuredValue),
			formatAlertValue(alert.RuleID, alert.Threshold))
	}

	report.WriteString("\nFinal positions\n")
	if len(result.Summary.Positions) == 0 {
		report.WriteString("  none\n")
	}
	for _, position := range result.Summary.Positions {
		mark := "unmarked"
		if position.LastMark != nil {
			mark = formatUSD(position.LastMark.String())
		}
		fmt.Fprintf(&report, "  %s qty=%s avg_cost=%s realized=%s mark=%s unrealized=%s notional=%s\n",
			position.Symbol, position.Quantity, formatUSD(position.AverageCost.String()),
			formatUSD(position.RealizedPnL.String()), mark,
			formatUSD(position.UnrealizedPnL.String()), formatUSD(position.Notional.String()))
	}

	fmt.Fprintf(&report, "\nTotals\n  processed_events=%d realized=%s unrealized=%s total_pnl=%s gross_notional=%s alerts=%d\n",
		result.Summary.ProcessedEvents, formatUSD(result.Summary.TotalRealizedPnL.String()),
		formatUSD(result.Summary.TotalUnrealizedPnL.String()), formatUSD(result.Summary.TotalPnL.String()),
		formatUSD(result.Summary.GrossNotional.String()), result.Summary.AlertsEmitted)
	if result.Summary.ProcessedEvents > 0 && result.Elapsed <= 0 {
		report.WriteString("  replay_throughput=unavailable (elapsed below timer resolution; use the benchmark)\n")
	} else {
		fmt.Fprintf(&report, "  replay_throughput=%.0f events/sec elapsed=%s\n",
			result.EventsPerSecond, result.Elapsed.Round(time.Microsecond))
	}
	_, err := io.WriteString(writer, report.String())
	return err
}

func formatAlertValue(ruleID, value string) string {
	if ruleID == RuleMaxGrossNotional {
		return formatUSD(value)
	}
	return value
}

func formatUSD(value string) string {
	if strings.HasPrefix(value, "-") {
		return "-$" + strings.TrimPrefix(value, "-")
	}
	return "$" + value
}
