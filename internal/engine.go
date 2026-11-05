package riskmon

import (
	"fmt"
	"sort"
)

// Engine is not safe for concurrent use.
type Engine struct {
	config             Config
	positions          map[string]Position
	limits             *LimitTracker
	grossNotional      MoneyCents
	totalRealizedPnL   MoneyCents
	totalUnrealizedPnL MoneyCents
	processedEvents    int64
	alerts             []Alert
}

func NewEngine(config Config) (*Engine, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("engine config: %w", err)
	}
	return &Engine{
		config:    config,
		positions: make(map[string]Position),
		limits:    NewLimitTracker(config.Limits),
	}, nil
}

func (engine *Engine) Apply(event Event) (ApplyResult, error) {
	if err := ValidateEvent(event, engine.config.Bounds); err != nil {
		return ApplyResult{}, fmt.Errorf("event %q: %w", event.ID, err)
	}

	current, found := engine.positions[event.Symbol]
	if !found {
		current = Position{Symbol: event.Symbol}
	}
	var updated Position
	var err error
	switch event.Type {
	case EventFill:
		updated, err = applyFill(current, event, engine.config.Bounds)
	case EventMark:
		updated, err = applyMark(current, event)
	default:
		err = fmt.Errorf("unsupported event type %q", event.Type)
	}
	if err != nil {
		return ApplyResult{}, fmt.Errorf("event %q for %s: %w", event.ID, event.Symbol, err)
	}

	newGross, err := replaceAggregate(int64(engine.grossNotional), int64(current.Notional), int64(updated.Notional))
	if err != nil {
		return ApplyResult{}, fmt.Errorf("event %q gross notional: %w", event.ID, err)
	}
	newRealized, err := replaceAggregate(int64(engine.totalRealizedPnL), int64(current.RealizedPnL), int64(updated.RealizedPnL))
	if err != nil {
		return ApplyResult{}, fmt.Errorf("event %q total realized pnl: %w", event.ID, err)
	}
	newUnrealized, err := replaceAggregate(int64(engine.totalUnrealizedPnL), int64(current.UnrealizedPnL), int64(updated.UnrealizedPnL))
	if err != nil {
		return ApplyResult{}, fmt.Errorf("event %q total unrealized pnl: %w", event.ID, err)
	}
	if _, err := checkedAdd(newRealized, newUnrealized); err != nil {
		return ApplyResult{}, fmt.Errorf("event %q total pnl: %w", event.ID, err)
	}
	processedEvents, err := checkedAdd(engine.processedEvents, 1)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("event count: %w", err)
	}

	// Commit only after all fallible accounting operations succeed.
	engine.positions[event.Symbol] = updated
	engine.grossNotional = MoneyCents(newGross)
	engine.totalRealizedPnL = MoneyCents(newRealized)
	engine.totalUnrealizedPnL = MoneyCents(newUnrealized)
	engine.processedEvents = processedEvents

	alerts := engine.limits.Evaluate(event.Symbol, updated.Quantity, engine.grossNotional, event.ID)
	engine.alerts = append(engine.alerts, alerts...)
	return ApplyResult{
		Event:              event,
		Position:           clonePosition(updated),
		GrossNotional:      engine.grossNotional,
		TotalRealizedPnL:   engine.totalRealizedPnL,
		TotalUnrealizedPnL: engine.totalUnrealizedPnL,
		Alerts:             append([]Alert(nil), alerts...),
	}, nil
}

func (engine *Engine) Position(symbol string) (Position, bool) {
	position, found := engine.positions[symbol]
	return clonePosition(position), found
}

func (engine *Engine) Alerts() []Alert {
	return append([]Alert(nil), engine.alerts...)
}

func (engine *Engine) Summary() (Summary, error) {
	symbols := make([]string, 0, len(engine.positions))
	for symbol := range engine.positions {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	positions := make([]Position, 0, len(symbols))
	for _, symbol := range symbols {
		positions = append(positions, clonePosition(engine.positions[symbol]))
	}
	totalPnL, err := checkedAdd(int64(engine.totalRealizedPnL), int64(engine.totalUnrealizedPnL))
	if err != nil {
		return Summary{}, fmt.Errorf("total pnl: %w", err)
	}
	return Summary{
		ProcessedEvents:    engine.processedEvents,
		Positions:          positions,
		TotalRealizedPnL:   engine.totalRealizedPnL,
		TotalUnrealizedPnL: engine.totalUnrealizedPnL,
		TotalPnL:           MoneyCents(totalPnL),
		GrossNotional:      engine.grossNotional,
		AlertsEmitted:      len(engine.alerts),
	}, nil
}

func replaceAggregate(total, oldValue, newValue int64) (int64, error) {
	delta, err := checkedSub(newValue, oldValue)
	if err != nil {
		return 0, err
	}
	return checkedAdd(total, delta)
}

func clonePosition(position Position) Position {
	if position.LastMark != nil {
		mark := *position.LastMark
		position.LastMark = &mark
	}
	return position
}
