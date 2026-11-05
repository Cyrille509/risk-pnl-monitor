package riskmon

import (
	"math"
	"reflect"
	"testing"
)

func TestLimitClassificationUsesInclusiveBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		measured int64
		want     LimitState
	}{
		{name: "below warning", measured: 999, want: StateNormal},
		{name: "at warning", measured: 1_000, want: StateWarn},
		{name: "below breach", measured: 1_999, want: StateWarn},
		{name: "at breach", measured: 2_000, want: StateBreach},
		{name: "above breach", measured: 2_001, want: StateBreach},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyLimit(test.measured, 1_000, 2_000); got != test.want {
				t.Fatalf("classifyLimit(%d) = %s, want %s", test.measured, got, test.want)
			}
		})
	}
}

func TestLimitTrackerPositionTransitionsAndNoDuplicates(t *testing.T) {
	tracker := newLimitTestTracker()

	if got := tracker.Evaluate("AAPL", limitTestQty("9.999"), limitTestMoney("0.00"), "e0"); len(got) != 0 {
		t.Fatalf("normal evaluation emitted alerts: %+v", got)
	}

	warn := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("10.000"), limitTestMoney("0.00"), "e1"))
	wantWarn := Alert{
		Kind:          AlertWarn,
		RuleID:        RuleMaxAbsPosition,
		Symbol:        "AAPL",
		PreviousState: StateNormal,
		NewState:      StateWarn,
		MeasuredValue: "10.000",
		Threshold:     "10.000",
		CausingEvent:  "e1",
	}
	assertLimitAlert(t, warn, wantWarn)

	if got := tracker.Evaluate("AAPL", limitTestQty("-15.000"), limitTestMoney("0.00"), "e2"); len(got) != 0 {
		t.Fatalf("same-state evaluation emitted alerts: %+v", got)
	}

	breach := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("-20.000"), limitTestMoney("0.00"), "e3"))
	wantBreach := Alert{
		Kind:          AlertBreach,
		RuleID:        RuleMaxAbsPosition,
		Symbol:        "AAPL",
		PreviousState: StateWarn,
		NewState:      StateBreach,
		MeasuredValue: "20.000",
		Threshold:     "20.000",
		CausingEvent:  "e3",
	}
	assertLimitAlert(t, breach, wantBreach)

	if got := tracker.Evaluate("AAPL", limitTestQty("25.000"), limitTestMoney("0.00"), "e4"); len(got) != 0 {
		t.Fatalf("duplicate breach emitted alerts: %+v", got)
	}

	clear := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("9.999"), limitTestMoney("0.00"), "e5"))
	wantClear := Alert{
		Kind:          AlertClear,
		RuleID:        RuleMaxAbsPosition,
		Symbol:        "AAPL",
		PreviousState: StateBreach,
		NewState:      StateNormal,
		MeasuredValue: "9.999",
		Threshold:     "10.000",
		CausingEvent:  "e5",
	}
	assertLimitAlert(t, clear, wantClear)
}

func TestLimitTrackerBreachToWarnUsesBreachThreshold(t *testing.T) {
	tracker := newLimitTestTracker()
	_ = tracker.Evaluate("AAPL", limitTestQty("20.000"), limitTestMoney("0.00"), "e1")

	alert := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("19.999"), limitTestMoney("0.00"), "e2"))
	assertLimitAlert(t, alert, Alert{
		Kind:          AlertWarn,
		RuleID:        RuleMaxAbsPosition,
		Symbol:        "AAPL",
		PreviousState: StateBreach,
		NewState:      StateWarn,
		MeasuredValue: "19.999",
		Threshold:     "20.000",
		CausingEvent:  "e2",
	})
}

func TestLimitTrackerDirectTransitions(t *testing.T) {
	t.Run("normal to breach", func(t *testing.T) {
		tracker := newLimitTestTracker()
		alert := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("30.000"), limitTestMoney("0.00"), "e1"))
		if alert.Kind != AlertBreach || alert.PreviousState != StateNormal || alert.NewState != StateBreach || alert.Threshold != "20.000" {
			t.Fatalf("unexpected direct breach: %+v", alert)
		}
	})

	t.Run("warn to normal", func(t *testing.T) {
		tracker := newLimitTestTracker()
		_ = tracker.Evaluate("AAPL", limitTestQty("10.000"), limitTestMoney("0.00"), "e1")
		alert := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("0.000"), limitTestMoney("0.00"), "e2"))
		if alert.Kind != AlertClear || alert.PreviousState != StateWarn || alert.NewState != StateNormal || alert.Threshold != "10.000" {
			t.Fatalf("unexpected direct clear: %+v", alert)
		}
	})
}

func TestLimitTrackerKeepsPositionStatePerSymbol(t *testing.T) {
	tracker := newLimitTestTracker()
	_ = tracker.Evaluate("AAPL", limitTestQty("10.000"), limitTestMoney("0.00"), "e1")

	msft := onlyLimitAlert(t, tracker.Evaluate("MSFT", limitTestQty("20.000"), limitTestMoney("0.00"), "e2"))
	if msft.Symbol != "MSFT" || msft.PreviousState != StateNormal || msft.NewState != StateBreach {
		t.Fatalf("MSFT inherited another symbol's state: %+v", msft)
	}

	aapl := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("20.000"), limitTestMoney("0.00"), "e3"))
	if aapl.Symbol != "AAPL" || aapl.PreviousState != StateWarn || aapl.NewState != StateBreach {
		t.Fatalf("AAPL state was not retained independently: %+v", aapl)
	}
}

func TestLimitTrackerGrossNotionalIsSingleton(t *testing.T) {
	tracker := newLimitTestTracker()

	warn := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("0.000"), limitTestMoney("100.00"), "e1"))
	assertLimitAlert(t, warn, Alert{
		Kind:          AlertWarn,
		RuleID:        RuleMaxGrossNotional,
		PreviousState: StateNormal,
		NewState:      StateWarn,
		MeasuredValue: "100.00",
		Threshold:     "100.00",
		CausingEvent:  "e1",
	})

	if got := tracker.Evaluate("MSFT", limitTestQty("0.000"), limitTestMoney("150.00"), "e2"); len(got) != 0 {
		t.Fatalf("gross state was not shared across symbols: %+v", got)
	}

	breach := onlyLimitAlert(t, tracker.Evaluate("MSFT", limitTestQty("0.000"), limitTestMoney("200.00"), "e3"))
	assertLimitAlert(t, breach, Alert{
		Kind:          AlertBreach,
		RuleID:        RuleMaxGrossNotional,
		PreviousState: StateWarn,
		NewState:      StateBreach,
		MeasuredValue: "200.00",
		Threshold:     "200.00",
		CausingEvent:  "e3",
	})

	downward := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("0.000"), limitTestMoney("199.99"), "e4"))
	assertLimitAlert(t, downward, Alert{
		Kind:          AlertWarn,
		RuleID:        RuleMaxGrossNotional,
		PreviousState: StateBreach,
		NewState:      StateWarn,
		MeasuredValue: "199.99",
		Threshold:     "200.00",
		CausingEvent:  "e4",
	})

	clear := onlyLimitAlert(t, tracker.Evaluate("AAPL", limitTestQty("0.000"), limitTestMoney("99.99"), "e5"))
	if clear.Kind != AlertClear || clear.RuleID != RuleMaxGrossNotional || clear.Threshold != "100.00" {
		t.Fatalf("unexpected gross clear: %+v", clear)
	}
}

func TestLimitTrackerOrdersSimultaneousAlerts(t *testing.T) {
	tracker := newLimitTestTracker()
	alerts := tracker.Evaluate("AAPL", limitTestQty("20.000"), limitTestMoney("200.00"), "e1")
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(alerts), alerts)
	}
	if alerts[0].RuleID != RuleMaxAbsPosition || alerts[1].RuleID != RuleMaxGrossNotional {
		t.Fatalf("unexpected alert order: %+v", alerts)
	}
}

func TestLimitTrackerClassifiesMinimumIntQuantityConservatively(t *testing.T) {
	tracker := newLimitTestTracker()
	alert := onlyLimitAlert(t, tracker.Evaluate("AAPL", QtyMilli(math.MinInt64), limitTestMoney("0.00"), "e1"))
	if alert.Kind != AlertBreach || alert.NewState != StateBreach || alert.MeasuredValue != "9223372036854775.808" {
		t.Fatalf("unexpected minimum-int alert: %+v", alert)
	}
}

func newLimitTestTracker() *LimitTracker {
	return NewLimitTracker(LimitsConfig{
		MaxAbsPosition: QtyLimitConfig{
			Warn:   limitTestQty("10.000"),
			Breach: limitTestQty("20.000"),
		},
		MaxGrossNotional: MoneyLimitConfig{
			Warn:   limitTestMoney("100.00"),
			Breach: limitTestMoney("200.00"),
		},
	})
}

func limitTestQty(value string) QtyMilli {
	parsed, err := ParseQuantity(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func limitTestMoney(value string) MoneyCents {
	parsed, err := ParseMoney(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func onlyLimitAlert(t *testing.T, alerts []Alert) Alert {
	t.Helper()
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1: %+v", len(alerts), alerts)
	}
	return alerts[0]
}

func assertLimitAlert(t *testing.T, got, want Alert) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("alert mismatch\n got: %+v\nwant: %+v", got, want)
	}
}
