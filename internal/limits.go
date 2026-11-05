package riskmon

import (
	"math"
	"strings"
)

// Position limits are tracked per symbol; the gross limit is shared.
type LimitTracker struct {
	config         LimitsConfig
	positionStates map[string]LimitState
	grossState     LimitState
}

func NewLimitTracker(config LimitsConfig) *LimitTracker {
	return &LimitTracker{
		config:         config,
		positionStates: make(map[string]LimitState),
		grossState:     StateNormal,
	}
}

// Evaluate emits state changes after an event, position before gross notional.
func (tracker *LimitTracker) Evaluate(
	symbol string,
	position QtyMilli,
	grossNotional MoneyCents,
	causingEventID string,
) []Alert {
	if tracker.positionStates == nil {
		tracker.positionStates = make(map[string]LimitState)
	}

	alerts := make([]Alert, 0, 2)
	positionValue, positionValueText := absoluteQuantity(position)
	previousPosition := normalizeState(tracker.positionStates[symbol])
	newPosition := classifyLimit(
		int64(positionValue),
		int64(tracker.config.MaxAbsPosition.Warn),
		int64(tracker.config.MaxAbsPosition.Breach),
	)
	if previousPosition != newPosition {
		measuredValue := positionValueText
		if measuredValue == "" {
			measuredValue = positionValue.String()
		}
		threshold := selectedThreshold(
			previousPosition,
			newPosition,
			tracker.config.MaxAbsPosition.Warn.String(),
			tracker.config.MaxAbsPosition.Breach.String(),
		)
		alerts = append(alerts, Alert{
			Kind:          kindForState(newPosition),
			RuleID:        RuleMaxAbsPosition,
			Symbol:        symbol,
			PreviousState: previousPosition,
			NewState:      newPosition,
			MeasuredValue: measuredValue,
			Threshold:     threshold,
			CausingEvent:  causingEventID,
		})
	}
	if newPosition == StateNormal {
		delete(tracker.positionStates, symbol)
	} else {
		tracker.positionStates[symbol] = newPosition
	}

	previousGross := normalizeState(tracker.grossState)
	newGross := classifyLimit(
		int64(grossNotional),
		int64(tracker.config.MaxGrossNotional.Warn),
		int64(tracker.config.MaxGrossNotional.Breach),
	)
	if previousGross != newGross {
		threshold := selectedThreshold(
			previousGross,
			newGross,
			tracker.config.MaxGrossNotional.Warn.String(),
			tracker.config.MaxGrossNotional.Breach.String(),
		)
		alerts = append(alerts, Alert{
			Kind:          kindForState(newGross),
			RuleID:        RuleMaxGrossNotional,
			PreviousState: previousGross,
			NewState:      newGross,
			MeasuredValue: grossNotional.String(),
			Threshold:     threshold,
			CausingEvent:  causingEventID,
		})
	}
	tracker.grossState = newGross

	return alerts
}

func classifyLimit(measured, warn, breach int64) LimitState {
	switch {
	case measured >= breach:
		return StateBreach
	case measured >= warn:
		return StateWarn
	default:
		return StateNormal
	}
}

func absoluteQuantity(quantity QtyMilli) (QtyMilli, string) {
	if quantity == QtyMilli(math.MinInt64) {
		// abs(MinInt64) overflows. Saturate for limit checks, but keep the exact text.
		return QtyMilli(math.MaxInt64), strings.TrimPrefix(quantity.String(), "-")
	}
	if quantity < 0 {
		quantity = -quantity
	}
	return quantity, ""
}

func normalizeState(state LimitState) LimitState {
	if state == "" {
		return StateNormal
	}
	return state
}

func kindForState(state LimitState) AlertKind {
	switch state {
	case StateNormal:
		return AlertClear
	case StateBreach:
		return AlertBreach
	default:
		return AlertWarn
	}
}

func selectedThreshold(previous, next LimitState, warn, breach string) string {
	switch {
	case next == StateNormal:
		return warn
	case previous == StateBreach && next == StateWarn:
		return breach
	case next == StateBreach:
		return breach
	default:
		return warn
	}
}
