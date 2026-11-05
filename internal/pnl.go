package riskmon

import "fmt"

// Return a copy so failed accounting leaves the original position unchanged.
func applyFill(position Position, event Event, bounds Bounds) (Position, error) {
	currentQuantity := int64(position.Quantity)
	deltaQuantity := int64(event.Quantity)
	newQuantity, err := checkedAdd(currentQuantity, deltaQuantity)
	if err != nil {
		return Position{}, fmt.Errorf("quantity update: %w", err)
	}
	absoluteNew, err := absInt64(newQuantity)
	if err != nil {
		return Position{}, fmt.Errorf("new quantity: %w", err)
	}
	if absoluteNew > int64(bounds.MaxAbsQuantity) {
		return Position{}, fmt.Errorf("resulting absolute position %s exceeds configured maximum %s", QtyMilli(absoluteNew), bounds.MaxAbsQuantity)
	}

	updated := position
	if currentQuantity == 0 || sameDirection(currentQuantity, deltaQuantity) {
		absoluteCurrent, err := absInt64(currentQuantity)
		if err != nil {
			return Position{}, fmt.Errorf("current quantity: %w", err)
		}
		absoluteDelta, err := absInt64(deltaQuantity)
		if err != nil {
			return Position{}, fmt.Errorf("fill quantity: %w", err)
		}
		oldCost, err := checkedMul(int64(position.AverageCost), absoluteCurrent)
		if err != nil {
			return Position{}, fmt.Errorf("weighted existing cost: %w", err)
		}
		fillCost, err := checkedMul(int64(event.Price), absoluteDelta)
		if err != nil {
			return Position{}, fmt.Errorf("weighted fill cost: %w", err)
		}
		totalCost, err := checkedAdd(oldCost, fillCost)
		if err != nil {
			return Position{}, fmt.Errorf("weighted total cost: %w", err)
		}
		totalQuantity, err := checkedAdd(absoluteCurrent, absoluteDelta)
		if err != nil {
			return Position{}, fmt.Errorf("weighted total quantity: %w", err)
		}
		updated.AverageCost = PriceCents(totalCost / totalQuantity)
		updated.Quantity = QtyMilli(newQuantity)
	} else {
		absoluteCurrent, err := absInt64(currentQuantity)
		if err != nil {
			return Position{}, fmt.Errorf("current quantity: %w", err)
		}
		absoluteDelta, err := absInt64(deltaQuantity)
		if err != nil {
			return Position{}, fmt.Errorf("fill quantity: %w", err)
		}
		closedQuantity := minInt64(absoluteCurrent, absoluteDelta)
		priceDifference, err := checkedSub(int64(event.Price), int64(position.AverageCost))
		if err != nil {
			return Position{}, fmt.Errorf("realized price difference: %w", err)
		}
		realizedNumerator, err := checkedMul(priceDifference, closedQuantity)
		if err != nil {
			return Position{}, fmt.Errorf("realized pnl product: %w", err)
		}
		if currentQuantity < 0 {
			realizedNumerator, err = checkedSub(0, realizedNumerator)
			if err != nil {
				return Position{}, fmt.Errorf("realized pnl sign: %w", err)
			}
		}
		realizedDelta := realizedNumerator / qtyScale
		newRealized, err := checkedAdd(int64(position.RealizedPnL), realizedDelta)
		if err != nil {
			return Position{}, fmt.Errorf("cumulative realized pnl: %w", err)
		}
		updated.RealizedPnL = MoneyCents(newRealized)
		updated.Quantity = QtyMilli(newQuantity)
		switch {
		case newQuantity == 0:
			updated.AverageCost = 0
		case sameDirection(currentQuantity, newQuantity):
			// A reduction keeps the original average cost.
		default:
			// The remainder after crossing zero opens at the fill price.
			updated.AverageCost = event.Price
		}
	}

	if err := revaluePosition(&updated); err != nil {
		return Position{}, err
	}
	return updated, nil
}

func applyMark(position Position, event Event) (Position, error) {
	updated := position
	mark := event.Price
	updated.LastMark = &mark
	if err := revaluePosition(&updated); err != nil {
		return Position{}, err
	}
	return updated, nil
}

func revaluePosition(position *Position) error {
	if position.Quantity == 0 || position.LastMark == nil {
		position.UnrealizedPnL = 0
		position.Notional = 0
		return nil
	}
	priceDifference, err := checkedSub(int64(*position.LastMark), int64(position.AverageCost))
	if err != nil {
		return fmt.Errorf("unrealized price difference: %w", err)
	}
	unrealizedNumerator, err := checkedMul(priceDifference, int64(position.Quantity))
	if err != nil {
		return fmt.Errorf("unrealized pnl product: %w", err)
	}
	position.UnrealizedPnL = MoneyCents(unrealizedNumerator / qtyScale)

	absoluteQuantity, err := absInt64(int64(position.Quantity))
	if err != nil {
		return fmt.Errorf("notional quantity: %w", err)
	}
	notionalNumerator, err := checkedMul(int64(*position.LastMark), absoluteQuantity)
	if err != nil {
		return fmt.Errorf("notional product: %w", err)
	}
	position.Notional = MoneyCents(notionalNumerator / qtyScale)
	return nil
}

func sameDirection(left, right int64) bool {
	return (left > 0 && right > 0) || (left < 0 && right < 0)
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
