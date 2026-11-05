package riskmon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	pricePrecision = 2
	qtyPrecision   = 3
	qtyScale       = int64(1_000)
)

// PriceCents stores a non-negative price in hundredths of a dollar.
type PriceCents int64

// QtyMilli stores a signed quantity in thousandths of a unit.
type QtyMilli int64

// MoneyCents stores a signed monetary value in hundredths of a dollar.
type MoneyCents int64

func ParsePrice(value string) (PriceCents, error) {
	parsed, err := parseFixed(value, pricePrecision)
	if err != nil {
		return 0, fmt.Errorf("price %q: %w", value, err)
	}
	return PriceCents(parsed), nil
}

func ParseQuantity(value string) (QtyMilli, error) {
	parsed, err := parseFixed(value, qtyPrecision)
	if err != nil {
		return 0, fmt.Errorf("quantity %q: %w", value, err)
	}
	return QtyMilli(parsed), nil
}

func ParseMoney(value string) (MoneyCents, error) {
	parsed, err := parseFixed(value, pricePrecision)
	if err != nil {
		return 0, fmt.Errorf("money %q: %w", value, err)
	}
	return MoneyCents(parsed), nil
}

func (value PriceCents) String() string { return formatFixed(int64(value), pricePrecision) }
func (value QtyMilli) String() string   { return formatFixed(int64(value), qtyPrecision) }
func (value MoneyCents) String() string { return formatFixed(int64(value), pricePrecision) }

func (value PriceCents) MarshalJSON() ([]byte, error) { return json.Marshal(value.String()) }
func (value QtyMilli) MarshalJSON() ([]byte, error)   { return json.Marshal(value.String()) }
func (value MoneyCents) MarshalJSON() ([]byte, error) { return json.Marshal(value.String()) }

func (value *PriceCents) UnmarshalJSON(data []byte) error {
	parsed, err := unmarshalFixed(data, pricePrecision, "price")
	if err != nil {
		return err
	}
	*value = PriceCents(parsed)
	return nil
}

func (value *QtyMilli) UnmarshalJSON(data []byte) error {
	parsed, err := unmarshalFixed(data, qtyPrecision, "quantity")
	if err != nil {
		return err
	}
	*value = QtyMilli(parsed)
	return nil
}

func (value *MoneyCents) UnmarshalJSON(data []byte) error {
	parsed, err := unmarshalFixed(data, pricePrecision, "money")
	if err != nil {
		return err
	}
	*value = MoneyCents(parsed)
	return nil
}

func unmarshalFixed(data []byte, precision int, kind string) (int64, error) {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return 0, fmt.Errorf("%s must be a decimal JSON string: %w", kind, err)
	}
	parsed, err := parseFixed(text, precision)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", kind, text, err)
	}
	return parsed, nil
}

func parseFixed(value string, precision int) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("value is empty")
	}
	if strings.TrimSpace(value) != value {
		return 0, fmt.Errorf("surrounding whitespace is not allowed")
	}

	negative := value[0] == '-'
	if negative {
		value = value[1:]
		if value == "" {
			return 0, fmt.Errorf("missing digits")
		}
	}
	if value[0] == '+' {
		return 0, fmt.Errorf("leading plus sign is not allowed")
	}
	if strings.Count(value, ".") > 1 {
		return 0, fmt.Errorf("multiple decimal points")
	}

	whole, fraction, hasPoint := strings.Cut(value, ".")
	if whole == "" {
		return 0, fmt.Errorf("digits are required before the decimal point")
	}
	if hasPoint && fraction == "" {
		return 0, fmt.Errorf("digits are required after the decimal point")
	}
	if len(fraction) > precision {
		return 0, fmt.Errorf("supports at most %d decimal places", precision)
	}
	if !asciiDigits(whole) || (fraction != "" && !asciiDigits(fraction)) {
		return 0, fmt.Errorf("only base-10 digits and one decimal point are allowed")
	}

	digits := whole + fraction + strings.Repeat("0", precision-len(fraction))
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return 0, nil
	}
	magnitude, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("scaled value overflows int64")
	}
	limit := uint64(math.MaxInt64)
	if negative {
		limit++
	}
	if magnitude > limit {
		return 0, fmt.Errorf("scaled value overflows int64")
	}
	if !negative {
		return int64(magnitude), nil
	}
	if magnitude == uint64(math.MaxInt64)+1 {
		return math.MinInt64, nil
	}
	return -int64(magnitude), nil
}

func asciiDigits(value string) bool {
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func formatFixed(value int64, precision int) string {
	negative := value < 0
	var magnitude uint64
	if negative {
		magnitude = uint64(-(value + 1)) + 1
	} else {
		magnitude = uint64(value)
	}
	scale := uint64(1)
	for range precision {
		scale *= 10
	}
	whole := magnitude / scale
	fraction := magnitude % scale
	prefix := ""
	if negative {
		prefix = "-"
	}
	return fmt.Sprintf("%s%d.%0*d", prefix, whole, precision, fraction)
}

type EventType string

const (
	EventFill EventType = "fill"
	EventMark EventType = "mark"
)

type Event struct {
	ID       string     `json:"id"`
	Type     EventType  `json:"type"`
	Symbol   string     `json:"symbol"`
	Quantity QtyMilli   `json:"quantity,omitempty"`
	Price    PriceCents `json:"price"`
}

type Bounds struct {
	MaxPrice       PriceCents `json:"max_price"`
	MaxAbsQuantity QtyMilli   `json:"max_abs_quantity"`
}

type QtyLimitConfig struct {
	Warn   QtyMilli `json:"warn"`
	Breach QtyMilli `json:"breach"`
}

type MoneyLimitConfig struct {
	Warn   MoneyCents `json:"warn"`
	Breach MoneyCents `json:"breach"`
}

type LimitsConfig struct {
	MaxAbsPosition   QtyLimitConfig   `json:"max_abs_position"`
	MaxGrossNotional MoneyLimitConfig `json:"max_gross_notional"`
}

type Config struct {
	Bounds Bounds       `json:"bounds"`
	Limits LimitsConfig `json:"limits"`
}

func DecodeConfig(reader io.Reader) (Config, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var config Config
	if err := decodeOneJSON(bytes.NewReader(data), &config); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := validateJSONSchema(data, configJSONSchema); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Bounds.MaxPrice <= 0 {
		return fmt.Errorf("bounds.max_price must be positive")
	}
	if config.Bounds.MaxAbsQuantity <= 0 {
		return fmt.Errorf("bounds.max_abs_quantity must be positive")
	}
	if _, err := checkedMul(int64(config.Bounds.MaxPrice), int64(config.Bounds.MaxAbsQuantity)); err != nil {
		return fmt.Errorf("configured max_price * max_abs_quantity is unsafe: %w", err)
	}
	position := config.Limits.MaxAbsPosition
	if position.Warn <= 0 || position.Breach <= 0 {
		return fmt.Errorf("limits.max_abs_position thresholds must be positive")
	}
	if position.Warn >= position.Breach {
		return fmt.Errorf("limits.max_abs_position.warn must be less than breach")
	}
	if position.Breach > config.Bounds.MaxAbsQuantity {
		return fmt.Errorf("limits.max_abs_position.breach exceeds bounds.max_abs_quantity")
	}
	gross := config.Limits.MaxGrossNotional
	if gross.Warn <= 0 || gross.Breach <= 0 {
		return fmt.Errorf("limits.max_gross_notional thresholds must be positive")
	}
	if gross.Warn >= gross.Breach {
		return fmt.Errorf("limits.max_gross_notional.warn must be less than breach")
	}
	return nil
}

func DecodeEvent(data []byte, bounds Bounds) (Event, error) {
	var raw struct {
		ID       string          `json:"id"`
		Type     EventType       `json:"type"`
		Symbol   string          `json:"symbol"`
		Quantity json.RawMessage `json:"quantity"`
		Price    *PriceCents     `json:"price"`
	}
	if err := decodeOneJSON(bytes.NewReader(data), &raw); err != nil {
		return Event{}, err
	}
	if err := validateJSONSchema(data, eventJSONSchema); err != nil {
		return Event{}, err
	}
	if raw.Price == nil {
		return Event{}, fmt.Errorf("price is required")
	}
	event := Event{ID: raw.ID, Type: raw.Type, Symbol: raw.Symbol, Price: *raw.Price}
	switch raw.Type {
	case EventFill:
		if len(raw.Quantity) == 0 || bytes.Equal(bytes.TrimSpace(raw.Quantity), []byte("null")) {
			return Event{}, fmt.Errorf("quantity is required for fill events")
		}
		if err := json.Unmarshal(raw.Quantity, &event.Quantity); err != nil {
			return Event{}, err
		}
	case EventMark:
		if len(raw.Quantity) != 0 {
			return Event{}, fmt.Errorf("quantity is not allowed for mark events")
		}
	default:
		return Event{}, fmt.Errorf("type must be %q or %q", EventFill, EventMark)
	}
	if err := ValidateEvent(event, bounds); err != nil {
		return Event{}, err
	}
	return event, nil
}

func ValidateEvent(event Event, bounds Bounds) error {
	if event.ID == "" || strings.TrimSpace(event.ID) != event.ID {
		return fmt.Errorf("id must be non-empty without surrounding whitespace")
	}
	if event.Symbol == "" || strings.TrimSpace(event.Symbol) != event.Symbol {
		return fmt.Errorf("symbol must be non-empty without surrounding whitespace")
	}
	if event.Price <= 0 {
		return fmt.Errorf("price must be positive")
	}
	if event.Price > bounds.MaxPrice {
		return fmt.Errorf("price %s exceeds configured maximum %s", event.Price, bounds.MaxPrice)
	}
	switch event.Type {
	case EventFill:
		if event.Quantity == 0 {
			return fmt.Errorf("fill quantity must be non-zero")
		}
		absolute, err := absInt64(int64(event.Quantity))
		if err != nil {
			return fmt.Errorf("fill quantity: %w", err)
		}
		if absolute > int64(bounds.MaxAbsQuantity) {
			return fmt.Errorf("absolute fill quantity %s exceeds configured maximum %s", QtyMilli(absolute), bounds.MaxAbsQuantity)
		}
	case EventMark:
		if event.Quantity != 0 {
			return fmt.Errorf("mark quantity must be zero")
		}
	default:
		return fmt.Errorf("type must be %q or %q", EventFill, EventMark)
	}
	return nil
}

func decodeOneJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

type jsonObjectSchema map[string]jsonObjectSchema

var eventJSONSchema = jsonObjectSchema{
	"id":       nil,
	"type":     nil,
	"symbol":   nil,
	"quantity": nil,
	"price":    nil,
}

var configJSONSchema = jsonObjectSchema{
	"bounds": {
		"max_price":        nil,
		"max_abs_quantity": nil,
	},
	"limits": {
		"max_abs_position": {
			"warn":   nil,
			"breach": nil,
		},
		"max_gross_notional": {
			"warn":   nil,
			"breach": nil,
		},
	},
}

// encoding/json accepts duplicate keys and case-insensitive field names; reject both.
func validateJSONSchema(data []byte, schema jsonObjectSchema) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	return walkJSONValue(decoder, schema)
}

func walkJSONValue(decoder *json.Decoder, schema jsonObjectSchema) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = struct{}{}
			var childSchema jsonObjectSchema
			if schema != nil {
				var known bool
				childSchema, known = schema[key]
				if !known {
					return fmt.Errorf("unknown field %q", key)
				}
			}
			if err := walkJSONValue(decoder, childSchema); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder, nil); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

type Position struct {
	Symbol        string      `json:"symbol"`
	Quantity      QtyMilli    `json:"quantity"`
	AverageCost   PriceCents  `json:"average_cost"`
	RealizedPnL   MoneyCents  `json:"realized_pnl"`
	LastMark      *PriceCents `json:"last_mark"`
	UnrealizedPnL MoneyCents  `json:"unrealized_pnl"`
	Notional      MoneyCents  `json:"notional"`
}

type LimitState string

const (
	StateNormal LimitState = "NORMAL"
	StateWarn   LimitState = "WARN"
	StateBreach LimitState = "BREACH"
)

type AlertKind string

const (
	AlertWarn   AlertKind = "WARN"
	AlertBreach AlertKind = "BREACH"
	AlertClear  AlertKind = "CLEAR"
)

const (
	RuleMaxAbsPosition   = "max_abs_position"
	RuleMaxGrossNotional = "max_gross_notional"
)

type Alert struct {
	Kind          AlertKind  `json:"kind"`
	RuleID        string     `json:"rule_id"`
	Symbol        string     `json:"symbol,omitempty"`
	PreviousState LimitState `json:"previous_state"`
	NewState      LimitState `json:"new_state"`
	MeasuredValue string     `json:"measured_value"`
	Threshold     string     `json:"threshold"`
	CausingEvent  string     `json:"causing_event_id"`
}

type ApplyResult struct {
	Event              Event
	Position           Position
	GrossNotional      MoneyCents
	TotalRealizedPnL   MoneyCents
	TotalUnrealizedPnL MoneyCents
	Alerts             []Alert
}

type Summary struct {
	ProcessedEvents    int64      `json:"processed_events"`
	Positions          []Position `json:"positions"`
	TotalRealizedPnL   MoneyCents `json:"total_realized_pnl"`
	TotalUnrealizedPnL MoneyCents `json:"total_unrealized_pnl"`
	TotalPnL           MoneyCents `json:"total_pnl"`
	GrossNotional      MoneyCents `json:"gross_notional"`
	AlertsEmitted      int        `json:"alerts_emitted"`
}

func checkedAdd(left, right int64) (int64, error) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, fmt.Errorf("int64 addition overflow")
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, fmt.Errorf("int64 addition overflow")
	}
	return left + right, nil
}

func checkedSub(left, right int64) (int64, error) {
	if right > 0 && left < math.MinInt64+right {
		return 0, fmt.Errorf("int64 subtraction overflow")
	}
	if right < 0 && left > math.MaxInt64+right {
		return 0, fmt.Errorf("int64 subtraction overflow")
	}
	return left - right, nil
}

func checkedMul(left, right int64) (int64, error) {
	if left == 0 || right == 0 {
		return 0, nil
	}
	if (left == math.MinInt64 && right == -1) || (right == math.MinInt64 && left == -1) {
		return 0, fmt.Errorf("int64 multiplication overflow")
	}
	product := left * right
	if product/right != left {
		return 0, fmt.Errorf("int64 multiplication overflow")
	}
	return product, nil
}

func absInt64(value int64) (int64, error) {
	if value == math.MinInt64 {
		return 0, fmt.Errorf("absolute value overflows int64")
	}
	if value < 0 {
		return -value, nil
	}
	return value, nil
}
