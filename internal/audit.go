package riskmon

import (
	"encoding/json"
	"fmt"
	"io"
)

// AuditWriter writes one deterministic event record followed by any alert
// transition records caused by that event.
type AuditWriter struct {
	encoder *json.Encoder
}

func NewAuditWriter(writer io.Writer) *AuditWriter {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &AuditWriter{encoder: encoder}
}

func (writer *AuditWriter) WriteResult(result ApplyResult) error {
	eventRecord := struct {
		RecordType         string     `json:"record_type"`
		Event              Event      `json:"event"`
		Position           Position   `json:"position"`
		GrossNotional      MoneyCents `json:"gross_notional"`
		TotalRealizedPnL   MoneyCents `json:"total_realized_pnl"`
		TotalUnrealizedPnL MoneyCents `json:"total_unrealized_pnl"`
	}{
		RecordType:         "event",
		Event:              result.Event,
		Position:           result.Position,
		GrossNotional:      result.GrossNotional,
		TotalRealizedPnL:   result.TotalRealizedPnL,
		TotalUnrealizedPnL: result.TotalUnrealizedPnL,
	}
	if err := writer.encoder.Encode(eventRecord); err != nil {
		return fmt.Errorf("encode event audit record: %w", err)
	}
	for _, alert := range result.Alerts {
		alertRecord := struct {
			RecordType string `json:"record_type"`
			Alert      Alert  `json:"alert"`
		}{RecordType: "alert", Alert: alert}
		if err := writer.encoder.Encode(alertRecord); err != nil {
			return fmt.Errorf("encode alert audit record: %w", err)
		}
	}
	return nil
}
