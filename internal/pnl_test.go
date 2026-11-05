package riskmon

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestPnLWorkedSequence(t *testing.T) {
	engine := newPnLTestEngine(t)
	steps := []struct {
		name  string
		event Event
		want  Position
	}{
		{
			name:  "open_and_increase/open",
			event: pnlFillEvent(t, "fill-1", "AAPL", "2.000", "100.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "", "0.00", "0.00"),
		},
		{
			name:  "open_and_increase/increase",
			event: pnlFillEvent(t, "fill-2", "AAPL", "3.000", "110.00"),
			want:  pnlExpectedPosition(t, "AAPL", "5.000", "106.00", "0.00", "", "0.00", "0.00"),
		},
		{
			name:  "partial_and_full_close/partial_close",
			event: pnlFillEvent(t, "fill-3", "AAPL", "-2.000", "120.00"),
			want:  pnlExpectedPosition(t, "AAPL", "3.000", "106.00", "28.00", "", "0.00", "0.00"),
		},
		{
			name:  "partial_and_full_close/full_close",
			event: pnlFillEvent(t, "fill-4", "AAPL", "-3.000", "90.00"),
			want:  pnlExpectedPosition(t, "AAPL", "0.000", "0.00", "-20.00", "", "0.00", "0.00"),
		},
		{
			name:  "flip_position/open_long",
			event: pnlFillEvent(t, "fill-5", "AAPL", "4.000", "105.00"),
			want:  pnlExpectedPosition(t, "AAPL", "4.000", "105.00", "-20.00", "", "0.00", "0.00"),
		},
		{
			name:  "flip_position/sell_through_zero",
			event: pnlFillEvent(t, "fill-6", "AAPL", "-6.000", "100.00"),
			want:  pnlExpectedPosition(t, "AAPL", "-2.000", "100.00", "-40.00", "", "0.00", "0.00"),
		},
	}

	for index, step := range steps {
		result, err := engine.Apply(step.event)
		if err != nil {
			t.Fatalf("apply %s: %v", step.name, err)
		}
		t.Run(step.name, func(t *testing.T) {
			pnlAssertAppliedState(t, engine, result, step.want, int64(index+1))
		})
	}
}

func TestPnLMarkToMarketLongAndShort(t *testing.T) {
	t.Run("mark_to_market/long", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		steps := []struct {
			event Event
			want  Position
		}{
			{
				event: pnlFillEvent(t, "long-open", "AAPL", "2.000", "100.00"),
				want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "", "0.00", "0.00"),
			},
			{
				event: pnlMarkEvent(t, "long-mark-up", "AAPL", "110.00"),
				want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "110.00", "20.00", "220.00"),
			},
			{
				event: pnlMarkEvent(t, "long-mark-down", "AAPL", "90.00"),
				want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "90.00", "-20.00", "180.00"),
			},
		}

		for index, step := range steps {
			result, err := engine.Apply(step.event)
			if err != nil {
				t.Fatalf("apply event %s: %v", step.event.ID, err)
			}
			pnlAssertAppliedState(t, engine, result, step.want, int64(index+1))
		}
	})

	t.Run("mark_to_market/short", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		steps := []struct {
			event Event
			want  Position
		}{
			{
				event: pnlFillEvent(t, "short-open", "AAPL", "-2.000", "100.00"),
				want:  pnlExpectedPosition(t, "AAPL", "-2.000", "100.00", "0.00", "", "0.00", "0.00"),
			},
			{
				event: pnlMarkEvent(t, "short-mark-down", "AAPL", "90.00"),
				want:  pnlExpectedPosition(t, "AAPL", "-2.000", "100.00", "0.00", "90.00", "20.00", "180.00"),
			},
			{
				event: pnlMarkEvent(t, "short-mark-up", "AAPL", "110.00"),
				want:  pnlExpectedPosition(t, "AAPL", "-2.000", "100.00", "0.00", "110.00", "-20.00", "220.00"),
			},
		}

		for index, step := range steps {
			result, err := engine.Apply(step.event)
			if err != nil {
				t.Fatalf("apply event %s: %v", step.event.ID, err)
			}
			pnlAssertAppliedState(t, engine, result, step.want, int64(index+1))
		}
	})
}

func TestPnLRecomputesValuationOnFillsAfterMark(t *testing.T) {
	engine := newPnLTestEngine(t)
	steps := []struct {
		event Event
		want  Position
	}{
		{
			event: pnlFillEvent(t, "open", "AAPL", "2.000", "100.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "", "0.00", "0.00"),
		},
		{
			event: pnlMarkEvent(t, "mark", "AAPL", "120.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "120.00", "40.00", "240.00"),
		},
		{
			event: pnlFillEvent(t, "increase", "AAPL", "1.000", "130.00"),
			want:  pnlExpectedPosition(t, "AAPL", "3.000", "110.00", "0.00", "120.00", "30.00", "360.00"),
		},
		{
			event: pnlFillEvent(t, "reduce", "AAPL", "-1.000", "115.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "110.00", "5.00", "120.00", "20.00", "240.00"),
		},
	}

	for index, step := range steps {
		result, err := engine.Apply(step.event)
		if err != nil {
			t.Fatalf("apply event %s: %v", step.event.ID, err)
		}
		pnlAssertAppliedState(t, engine, result, step.want, int64(index+1))
	}
}

func TestPnLPreservesMarkAcrossFlat(t *testing.T) {
	engine := newPnLTestEngine(t)
	steps := []struct {
		event Event
		want  Position
	}{
		{
			event: pnlFillEvent(t, "open-long", "AAPL", "2.000", "100.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "", "0.00", "0.00"),
		},
		{
			event: pnlMarkEvent(t, "mark", "AAPL", "120.00"),
			want:  pnlExpectedPosition(t, "AAPL", "2.000", "100.00", "0.00", "120.00", "40.00", "240.00"),
		},
		{
			event: pnlFillEvent(t, "close", "AAPL", "-2.000", "110.00"),
			want:  pnlExpectedPosition(t, "AAPL", "0.000", "0.00", "20.00", "120.00", "0.00", "0.00"),
		},
		{
			event: pnlFillEvent(t, "reopen-short", "AAPL", "-1.000", "100.00"),
			want:  pnlExpectedPosition(t, "AAPL", "-1.000", "100.00", "20.00", "120.00", "-20.00", "120.00"),
		},
	}

	for index, step := range steps {
		result, err := engine.Apply(step.event)
		if err != nil {
			t.Fatalf("apply event %s: %v", step.event.ID, err)
		}
		pnlAssertAppliedState(t, engine, result, step.want, int64(index+1))
	}
}

func TestPnLDivisionTruncatesTowardZero(t *testing.T) {
	t.Run("weighted_average", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		first := pnlFillEvent(t, "first", "AAPL", "0.001", "100.00")
		result, err := engine.Apply(first)
		if err != nil {
			t.Fatalf("apply first fill: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.001", "100.00", "0.00", "", "0.00", "0.00"), 1)

		second := pnlFillEvent(t, "second", "AAPL", "0.002", "100.01")
		result, err = engine.Apply(second)
		if err != nil {
			t.Fatalf("apply second fill: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.003", "100.00", "0.00", "", "0.00", "0.00"), 2)
	})

	t.Run("positive_realized_pnl", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		result, err := engine.Apply(pnlFillEvent(t, "open", "AAPL", "0.500", "100.00"))
		if err != nil {
			t.Fatalf("open position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.500", "100.00", "0.00", "", "0.00", "0.00"), 1)

		result, err = engine.Apply(pnlFillEvent(t, "close", "AAPL", "-0.500", "100.03"))
		if err != nil {
			t.Fatalf("close position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.000", "0.00", "0.01", "", "0.00", "0.00"), 2)
	})

	t.Run("negative_realized_pnl", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		result, err := engine.Apply(pnlFillEvent(t, "open", "AAPL", "0.500", "100.00"))
		if err != nil {
			t.Fatalf("open position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.500", "100.00", "0.00", "", "0.00", "0.00"), 1)

		result, err = engine.Apply(pnlFillEvent(t, "close", "AAPL", "-0.500", "99.97"))
		if err != nil {
			t.Fatalf("close position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.000", "0.00", "-0.01", "", "0.00", "0.00"), 2)
	})

	t.Run("negative_unrealized_pnl_and_notional", func(t *testing.T) {
		engine := newPnLTestEngine(t)
		result, err := engine.Apply(pnlFillEvent(t, "open", "AAPL", "0.500", "100.00"))
		if err != nil {
			t.Fatalf("open position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.500", "100.00", "0.00", "", "0.00", "0.00"), 1)

		result, err = engine.Apply(pnlMarkEvent(t, "mark", "AAPL", "99.97"))
		if err != nil {
			t.Fatalf("mark position: %v", err)
		}
		pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "0.500", "100.00", "0.00", "99.97", "-0.01", "49.98"), 2)
	})
}

func TestPnLPositionAndBoundsRejectionIsTransactional(t *testing.T) {
	config := pnlTestConfig()
	config.Bounds.MaxPrice = pnlMustPrice(t, "1000.00")
	config.Bounds.MaxAbsQuantity = pnlMustQuantity(t, "5.000")
	config.Limits.MaxAbsPosition.Warn = pnlMustQuantity(t, "4.500")
	config.Limits.MaxAbsPosition.Breach = pnlMustQuantity(t, "5.000")

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	result, err := engine.Apply(pnlFillEvent(t, "baseline", "AAPL", "4.000", "100.00"))
	if err != nil {
		t.Fatalf("apply baseline: %v", err)
	}
	pnlAssertAppliedState(t, engine, result, pnlExpectedPosition(t, "AAPL", "4.000", "100.00", "0.00", "", "0.00", "0.00"), 1)

	beforePosition, found := engine.Position("AAPL")
	if !found {
		t.Fatal("baseline position is missing")
	}
	beforeSummary, err := engine.Summary()
	if err != nil {
		t.Fatalf("baseline summary: %v", err)
	}
	beforeAlerts := engine.Alerts()

	tests := []struct {
		name      string
		event     Event
		wantError string
	}{
		{
			name:      "resulting_position_exceeds_bound",
			event:     pnlFillEvent(t, "too-large-position", "AAPL", "2.000", "100.00"),
			wantError: "resulting absolute position",
		},
		{
			name:      "fill_quantity_exceeds_bound",
			event:     pnlFillEvent(t, "too-large-fill", "AAPL", "6.000", "100.00"),
			wantError: "absolute fill quantity",
		},
		{
			name:      "price_exceeds_bound",
			event:     pnlFillEvent(t, "too-large-price", "AAPL", "1.000", "1000.01"),
			wantError: "exceeds configured maximum",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failedResult, applyErr := engine.Apply(test.event)
			if applyErr == nil {
				t.Fatalf("Apply(%s) succeeded, want error", test.event.ID)
			}
			if !strings.Contains(applyErr.Error(), test.wantError) {
				t.Fatalf("Apply(%s) error = %q, want substring %q", test.event.ID, applyErr, test.wantError)
			}
			if !reflect.DeepEqual(failedResult, ApplyResult{}) {
				t.Fatalf("failed Apply returned non-zero result: %+v", failedResult)
			}

			afterPosition, afterFound := engine.Position("AAPL")
			if !afterFound || !reflect.DeepEqual(afterPosition, beforePosition) {
				t.Fatalf("position changed after rejected event\n got: %+v\nwant: %+v", afterPosition, beforePosition)
			}
			afterSummary, summaryErr := engine.Summary()
			if summaryErr != nil {
				t.Fatalf("summary after rejected event: %v", summaryErr)
			}
			if !reflect.DeepEqual(afterSummary, beforeSummary) {
				t.Fatalf("summary changed after rejected event\n got: %+v\nwant: %+v", afterSummary, beforeSummary)
			}
			if afterAlerts := engine.Alerts(); !reflect.DeepEqual(afterAlerts, beforeAlerts) {
				t.Fatalf("alerts changed after rejected event\n got: %+v\nwant: %+v", afterAlerts, beforeAlerts)
			}
		})
	}
}

func TestPnLCumulativeRealizedOverflowIsTransactional(t *testing.T) {
	config := Config{
		Bounds: Bounds{
			MaxPrice:       PriceCents(3_000_000_000),
			MaxAbsQuantity: QtyMilli(3_000_000_000),
		},
		Limits: LimitsConfig{
			MaxAbsPosition: QtyLimitConfig{
				Warn:   QtyMilli(2_500_000_000),
				Breach: QtyMilli(2_900_000_000),
			},
			MaxGrossNotional: MoneyLimitConfig{
				Warn:   MoneyCents(math.MaxInt64 - 1),
				Breach: MoneyCents(math.MaxInt64),
			},
		},
	}
	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("new overflow-test engine: %v", err)
	}

	const (
		tradeQuantity = QtyMilli(2_000_000_000)
		openPrice     = PriceCents(1)
		closePrice    = PriceCents(3_000_000_000)
	)
	for cycle := 0; cycle < 2_000; cycle++ {
		open := Event{
			ID:       fmt.Sprintf("open-%d", cycle),
			Type:     EventFill,
			Symbol:   "AAPL",
			Quantity: tradeQuantity,
			Price:    openPrice,
		}
		if _, applyErr := engine.Apply(open); applyErr != nil {
			t.Fatalf("cycle %d open failed before expected overflow: %v", cycle, applyErr)
		}

		beforePosition, found := engine.Position("AAPL")
		if !found {
			t.Fatalf("cycle %d open did not create a position", cycle)
		}
		beforeSummary, summaryErr := engine.Summary()
		if summaryErr != nil {
			t.Fatalf("cycle %d summary before close: %v", cycle, summaryErr)
		}
		beforeAlerts := engine.Alerts()

		closeEvent := Event{
			ID:       fmt.Sprintf("close-%d", cycle),
			Type:     EventFill,
			Symbol:   "AAPL",
			Quantity: -tradeQuantity,
			Price:    closePrice,
		}
		failedResult, applyErr := engine.Apply(closeEvent)
		if applyErr == nil {
			continue
		}
		if !strings.Contains(applyErr.Error(), "cumulative realized pnl: int64 addition overflow") {
			t.Fatalf("cycle %d close error = %q, want cumulative realized overflow", cycle, applyErr)
		}
		if !reflect.DeepEqual(failedResult, ApplyResult{}) {
			t.Fatalf("overflowing Apply returned non-zero result: %+v", failedResult)
		}

		afterPosition, afterFound := engine.Position("AAPL")
		if !afterFound || !reflect.DeepEqual(afterPosition, beforePosition) {
			t.Fatalf("position changed after overflow\n got: %+v\nwant: %+v", afterPosition, beforePosition)
		}
		afterSummary, summaryErr := engine.Summary()
		if summaryErr != nil {
			t.Fatalf("summary after overflow: %v", summaryErr)
		}
		if !reflect.DeepEqual(afterSummary, beforeSummary) {
			t.Fatalf("summary changed after overflow\n got: %+v\nwant: %+v", afterSummary, beforeSummary)
		}
		if afterAlerts := engine.Alerts(); !reflect.DeepEqual(afterAlerts, beforeAlerts) {
			t.Fatalf("alerts changed after overflow\n got: %+v\nwant: %+v", afterAlerts, beforeAlerts)
		}
		return
	}

	t.Fatal("expected cumulative realized PnL to overflow within 2000 close events")
}

func TestPnLCombinedTotalOverflowIsTransactional(t *testing.T) {
	config := Config{
		Bounds: Bounds{
			MaxPrice:       PriceCents(3_000_000_000),
			MaxAbsQuantity: QtyMilli(3_000_000_000),
		},
		Limits: LimitsConfig{
			MaxAbsPosition: QtyLimitConfig{
				Warn:   QtyMilli(2_500_000_000),
				Breach: QtyMilli(2_900_000_000),
			},
			MaxGrossNotional: MoneyLimitConfig{
				Warn:   MoneyCents(math.MaxInt64 - 1),
				Breach: MoneyCents(math.MaxInt64),
			},
		},
	}
	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("new total-overflow engine: %v", err)
	}

	const tradeQuantity = QtyMilli(2_000_000_000)
	for cycle := range 1_537 {
		if _, err := engine.Apply(Event{
			ID:       fmt.Sprintf("realized-open-%d", cycle),
			Type:     EventFill,
			Symbol:   "REALIZED",
			Quantity: tradeQuantity,
			Price:    1,
		}); err != nil {
			t.Fatalf("cycle %d open: %v", cycle, err)
		}
		if _, err := engine.Apply(Event{
			ID:       fmt.Sprintf("realized-close-%d", cycle),
			Type:     EventFill,
			Symbol:   "REALIZED",
			Quantity: -tradeQuantity,
			Price:    3_000_000_000,
		}); err != nil {
			t.Fatalf("cycle %d close: %v", cycle, err)
		}
	}
	if _, err := engine.Apply(Event{
		ID:       "unrealized-open",
		Type:     EventFill,
		Symbol:   "UNREALIZED",
		Quantity: tradeQuantity,
		Price:    1,
	}); err != nil {
		t.Fatalf("open position for unrealized pnl: %v", err)
	}

	beforePosition, found := engine.Position("UNREALIZED")
	if !found {
		t.Fatal("unrealized test position is missing")
	}
	beforeSummary, err := engine.Summary()
	if err != nil {
		t.Fatalf("summary before total overflow: %v", err)
	}
	beforeAlerts := engine.Alerts()
	overflowingMark := Event{
		ID:     "unrealized-mark",
		Type:   EventMark,
		Symbol: "UNREALIZED",
		Price:  1_000_000_000,
	}
	failedResult, applyErr := engine.Apply(overflowingMark)
	if applyErr == nil || !strings.Contains(applyErr.Error(), "total pnl: int64 addition overflow") {
		t.Fatalf("overflowing mark error = %v", applyErr)
	}
	if !reflect.DeepEqual(failedResult, ApplyResult{}) {
		t.Fatalf("overflowing Apply returned non-zero result: %+v", failedResult)
	}
	afterPosition, afterFound := engine.Position("UNREALIZED")
	if !afterFound || !reflect.DeepEqual(afterPosition, beforePosition) {
		t.Fatalf("position changed after total overflow\n got: %+v\nwant: %+v", afterPosition, beforePosition)
	}
	afterSummary, err := engine.Summary()
	if err != nil {
		t.Fatalf("summary after rejected total overflow: %v", err)
	}
	if !reflect.DeepEqual(afterSummary, beforeSummary) {
		t.Fatalf("summary changed after total overflow\n got: %+v\nwant: %+v", afterSummary, beforeSummary)
	}
	if afterAlerts := engine.Alerts(); !reflect.DeepEqual(afterAlerts, beforeAlerts) {
		t.Fatalf("alerts changed after total overflow\n got: %+v\nwant: %+v", afterAlerts, beforeAlerts)
	}
}

func pnlTestConfig() Config {
	return Config{
		Bounds: Bounds{
			MaxPrice:       PriceCents(1_000_000),
			MaxAbsQuantity: QtyMilli(1_000_000),
		},
		Limits: LimitsConfig{
			MaxAbsPosition: QtyLimitConfig{
				Warn:   QtyMilli(900_000),
				Breach: QtyMilli(950_000),
			},
			MaxGrossNotional: MoneyLimitConfig{
				Warn:   MoneyCents(900_000_000),
				Breach: MoneyCents(950_000_000),
			},
		},
	}
}

func newPnLTestEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(pnlTestConfig())
	if err != nil {
		t.Fatalf("new test engine: %v", err)
	}
	return engine
}

func pnlFillEvent(t *testing.T, id, symbol, quantity, price string) Event {
	t.Helper()
	return Event{
		ID:       id,
		Type:     EventFill,
		Symbol:   symbol,
		Quantity: pnlMustQuantity(t, quantity),
		Price:    pnlMustPrice(t, price),
	}
}

func pnlMarkEvent(t *testing.T, id, symbol, price string) Event {
	t.Helper()
	return Event{
		ID:     id,
		Type:   EventMark,
		Symbol: symbol,
		Price:  pnlMustPrice(t, price),
	}
}

func pnlExpectedPosition(t *testing.T, symbol, quantity, averageCost, realizedPnL, lastMark, unrealizedPnL, notional string) Position {
	t.Helper()
	position := Position{
		Symbol:        symbol,
		Quantity:      pnlMustQuantity(t, quantity),
		AverageCost:   pnlMustPrice(t, averageCost),
		RealizedPnL:   pnlMustMoney(t, realizedPnL),
		UnrealizedPnL: pnlMustMoney(t, unrealizedPnL),
		Notional:      pnlMustMoney(t, notional),
	}
	if lastMark != "" {
		mark := pnlMustPrice(t, lastMark)
		position.LastMark = &mark
	}
	return position
}

func pnlAssertAppliedState(t *testing.T, engine *Engine, result ApplyResult, want Position, processedEvents int64) {
	t.Helper()
	if !reflect.DeepEqual(result.Position, want) {
		t.Fatalf("Apply result position mismatch\n got: %+v\nwant: %+v", result.Position, want)
	}
	if len(result.Alerts) != 0 {
		t.Fatalf("unexpected alerts: %+v", result.Alerts)
	}
	if result.GrossNotional != want.Notional {
		t.Fatalf("gross notional = %s, want %s", result.GrossNotional, want.Notional)
	}
	if result.TotalRealizedPnL != want.RealizedPnL {
		t.Fatalf("total realized PnL = %s, want %s", result.TotalRealizedPnL, want.RealizedPnL)
	}
	if result.TotalUnrealizedPnL != want.UnrealizedPnL {
		t.Fatalf("total unrealized PnL = %s, want %s", result.TotalUnrealizedPnL, want.UnrealizedPnL)
	}

	stored, found := engine.Position(want.Symbol)
	if !found {
		t.Fatalf("position %q was not stored", want.Symbol)
	}
	if !reflect.DeepEqual(stored, want) {
		t.Fatalf("stored position mismatch\n got: %+v\nwant: %+v", stored, want)
	}

	summary, err := engine.Summary()
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	wantTotalPnL := MoneyCents(int64(want.RealizedPnL) + int64(want.UnrealizedPnL))
	wantSummary := Summary{
		ProcessedEvents:    processedEvents,
		Positions:          []Position{want},
		TotalRealizedPnL:   want.RealizedPnL,
		TotalUnrealizedPnL: want.UnrealizedPnL,
		TotalPnL:           wantTotalPnL,
		GrossNotional:      want.Notional,
		AlertsEmitted:      0,
	}
	if !reflect.DeepEqual(summary, wantSummary) {
		t.Fatalf("summary mismatch\n got: %+v\nwant: %+v", summary, wantSummary)
	}
}

func pnlMustPrice(t *testing.T, value string) PriceCents {
	t.Helper()
	parsed, err := ParsePrice(value)
	if err != nil {
		t.Fatalf("ParsePrice(%q): %v", value, err)
	}
	return parsed
}

func pnlMustQuantity(t *testing.T, value string) QtyMilli {
	t.Helper()
	parsed, err := ParseQuantity(value)
	if err != nil {
		t.Fatalf("ParseQuantity(%q): %v", value, err)
	}
	return parsed
}

func pnlMustMoney(t *testing.T, value string) MoneyCents {
	t.Helper()
	parsed, err := ParseMoney(value)
	if err != nil {
		t.Fatalf("ParseMoney(%q): %v", value, err)
	}
	return parsed
}
