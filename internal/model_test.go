package riskmon

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestFixedPointParseAndFormat(t *testing.T) {
	t.Run("price", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			want     PriceCents
			wantText string
		}{
			{name: "zero", input: "0", want: 0, wantText: "0.00"},
			{name: "integer", input: "12", want: 1_200, wantText: "12.00"},
			{name: "one decimal place", input: "12.3", want: 1_230, wantText: "12.30"},
			{name: "leading zeroes", input: "00012.30", want: 1_230, wantText: "12.30"},
			{name: "negative", input: "-0.01", want: -1, wantText: "-0.01"},
			{name: "negative zero canonicalizes", input: "-0.00", want: 0, wantText: "0.00"},
			{name: "max int64", input: "92233720368547758.07", want: PriceCents(math.MaxInt64), wantText: "92233720368547758.07"},
			{name: "min int64", input: "-92233720368547758.08", want: PriceCents(math.MinInt64), wantText: "-92233720368547758.08"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := ParsePrice(test.input)
				if err != nil {
					t.Fatalf("ParsePrice(%q) returned an error: %v", test.input, err)
				}
				if got != test.want {
					t.Fatalf("ParsePrice(%q) = %d, want %d", test.input, got, test.want)
				}
				if gotText := got.String(); gotText != test.wantText {
					t.Errorf("PriceCents(%d).String() = %q, want %q", got, gotText, test.wantText)
				}
			})
		}
	})

	t.Run("quantity", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			want     QtyMilli
			wantText string
		}{
			{name: "zero", input: "0", want: 0, wantText: "0.000"},
			{name: "integer", input: "12", want: 12_000, wantText: "12.000"},
			{name: "one decimal place", input: "12.3", want: 12_300, wantText: "12.300"},
			{name: "two decimal places", input: "12.34", want: 12_340, wantText: "12.340"},
			{name: "leading zeroes", input: "00012.340", want: 12_340, wantText: "12.340"},
			{name: "negative", input: "-0.001", want: -1, wantText: "-0.001"},
			{name: "negative zero canonicalizes", input: "-0.000", want: 0, wantText: "0.000"},
			{name: "max int64", input: "9223372036854775.807", want: QtyMilli(math.MaxInt64), wantText: "9223372036854775.807"},
			{name: "min int64", input: "-9223372036854775.808", want: QtyMilli(math.MinInt64), wantText: "-9223372036854775.808"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := ParseQuantity(test.input)
				if err != nil {
					t.Fatalf("ParseQuantity(%q) returned an error: %v", test.input, err)
				}
				if got != test.want {
					t.Fatalf("ParseQuantity(%q) = %d, want %d", test.input, got, test.want)
				}
				if gotText := got.String(); gotText != test.wantText {
					t.Errorf("QtyMilli(%d).String() = %q, want %q", got, gotText, test.wantText)
				}
			})
		}
	})

	t.Run("money", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			want     MoneyCents
			wantText string
		}{
			{name: "positive", input: "123.45", want: 12_345, wantText: "123.45"},
			{name: "negative", input: "-123.45", want: -12_345, wantText: "-123.45"},
			{name: "max int64", input: "92233720368547758.07", want: MoneyCents(math.MaxInt64), wantText: "92233720368547758.07"},
			{name: "min int64", input: "-92233720368547758.08", want: MoneyCents(math.MinInt64), wantText: "-92233720368547758.08"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := ParseMoney(test.input)
				if err != nil {
					t.Fatalf("ParseMoney(%q) returned an error: %v", test.input, err)
				}
				if got != test.want {
					t.Fatalf("ParseMoney(%q) = %d, want %d", test.input, got, test.want)
				}
				if gotText := got.String(); gotText != test.wantText {
					t.Errorf("MoneyCents(%d).String() = %q, want %q", got, gotText, test.wantText)
				}
			})
		}
	})
}

func TestFixedPointJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "price max int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, PriceCents(math.MaxInt64), `"92233720368547758.07"`)
			},
		},
		{
			name: "price min int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, PriceCents(math.MinInt64), `"-92233720368547758.08"`)
			},
		},
		{
			name: "quantity max int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, QtyMilli(math.MaxInt64), `"9223372036854775.807"`)
			},
		},
		{
			name: "quantity min int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, QtyMilli(math.MinInt64), `"-9223372036854775.808"`)
			},
		},
		{
			name: "money max int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, MoneyCents(math.MaxInt64), `"92233720368547758.07"`)
			},
		},
		{
			name: "money min int64",
			run: func(t *testing.T) {
				assertJSONRoundTrip(t, MoneyCents(math.MinInt64), `"-92233720368547758.08"`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestFixedPointParsingRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		parse  func(string) error
		values []string
	}{
		{
			name: "price",
			parse: func(value string) error {
				_, err := ParsePrice(value)
				return err
			},
			values: append([]string{
				"1.001",
				"92233720368547758.08",
				"-92233720368547758.09",
			}, malformedFixedPointValues()...),
		},
		{
			name: "quantity",
			parse: func(value string) error {
				_, err := ParseQuantity(value)
				return err
			},
			values: append([]string{
				"1.0001",
				"9223372036854775.808",
				"-9223372036854775.809",
			}, malformedFixedPointValues()...),
		},
		{
			name: "money",
			parse: func(value string) error {
				_, err := ParseMoney(value)
				return err
			},
			values: append([]string{
				"1.001",
				"92233720368547758.08",
				"-92233720368547758.09",
			}, malformedFixedPointValues()...),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for index, value := range test.values {
				t.Run(testName(index, value), func(t *testing.T) {
					if err := test.parse(value); err == nil {
						t.Fatalf("parsing %q unexpectedly succeeded", value)
					}
				})
			}
		})
	}
}

func TestFixedPointJSONRejectsNonStringAndInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		unmarshal func([]byte) error
		extra     string
	}{
		{
			name: "price",
			unmarshal: func(data []byte) error {
				var value PriceCents
				return json.Unmarshal(data, &value)
			},
			extra: `"1.001"`,
		},
		{
			name: "quantity",
			unmarshal: func(data []byte) error {
				var value QtyMilli
				return json.Unmarshal(data, &value)
			},
			extra: `"1.0001"`,
		},
		{
			name: "money",
			unmarshal: func(data []byte) error {
				var value MoneyCents
				return json.Unmarshal(data, &value)
			},
			extra: `"1.001"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := []string{
				`1.23`,
				`123`,
				`null`,
				`true`,
				`{}`,
				`[]`,
				`" 1.23"`,
				`"1.23 "`,
				`"\t1.23"`,
				`"+1.23"`,
				`"1e3"`,
				`"1..2"`,
				test.extra,
			}
			for index, value := range values {
				t.Run(testName(index, value), func(t *testing.T) {
					if err := test.unmarshal([]byte(value)); err == nil {
						t.Fatalf("unmarshalling %s unexpectedly succeeded", value)
					}
				})
			}
		})
	}
}

func TestDecodeEventValidSchemas(t *testing.T) {
	bounds := Bounds{MaxPrice: 100_00, MaxAbsQuantity: 10_000}

	t.Run("fill", func(t *testing.T) {
		got, err := DecodeEvent([]byte(`{
			"id":"fill-1",
			"type":"fill",
			"symbol":"AAPL",
			"quantity":"-2.500",
			"price":"12.34"
		}`), bounds)
		if err != nil {
			t.Fatalf("DecodeEvent returned an error: %v", err)
		}
		want := Event{
			ID:       "fill-1",
			Type:     EventFill,
			Symbol:   "AAPL",
			Quantity: -2_500,
			Price:    1_234,
		}
		if got != want {
			t.Fatalf("DecodeEvent() = %+v, want %+v", got, want)
		}
	})

	t.Run("mark", func(t *testing.T) {
		got, err := DecodeEvent([]byte(`{
			"id":"mark-1",
			"type":"mark",
			"symbol":"AAPL",
			"price":"99.99"
		}`), bounds)
		if err != nil {
			t.Fatalf("DecodeEvent returned an error: %v", err)
		}
		want := Event{ID: "mark-1", Type: EventMark, Symbol: "AAPL", Price: 9_999}
		if got != want {
			t.Fatalf("DecodeEvent() = %+v, want %+v", got, want)
		}
	})
}

func TestDecodeEventRejectsInvalidSchema(t *testing.T) {
	bounds := Bounds{MaxPrice: 100_00, MaxAbsQuantity: 10_000}
	validFill := `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "unknown field", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34","extra":true}`, want: `unknown field "extra"`},
		{name: "case-sensitive field name", data: `{"ID":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: `unknown field "ID"`},
		{name: "duplicate field", data: `{"id":"fill-1","id":"fill-2","type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: `duplicate field "id"`},
		{name: "second JSON value", data: validFill + `{}`, want: "multiple JSON values"},
		{name: "trailing garbage", data: validFill + ` garbage`, want: "trailing data"},
		{name: "malformed JSON", data: `{"id":`, want: "unexpected EOF"},
		{name: "top-level array", data: `[]`, want: "cannot unmarshal array"},
		{name: "missing id", data: `{"type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "id must be non-empty"},
		{name: "null id", data: `{"id":null,"type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "id must be non-empty"},
		{name: "id with surrounding whitespace", data: `{"id":" fill-1 ","type":"fill","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "id must be non-empty"},
		{name: "missing symbol", data: `{"id":"fill-1","type":"fill","quantity":"1.000","price":"12.34"}`, want: "symbol must be non-empty"},
		{name: "null symbol", data: `{"id":"fill-1","type":"fill","symbol":null,"quantity":"1.000","price":"12.34"}`, want: "symbol must be non-empty"},
		{name: "symbol with surrounding whitespace", data: `{"id":"fill-1","type":"fill","symbol":" AAPL ","quantity":"1.000","price":"12.34"}`, want: "symbol must be non-empty"},
		{name: "missing type", data: `{"id":"fill-1","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "type must be"},
		{name: "invalid type", data: `{"id":"fill-1","type":"trade","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "type must be"},
		{name: "missing price", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000"}`, want: "price is required"},
		{name: "null price", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":null}`, want: "price is required"},
		{name: "numeric price", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":12.34}`, want: "decimal JSON string"},
		{name: "zero price", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"0.00"}`, want: "price must be positive"},
		{name: "negative price", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"-0.01"}`, want: "price must be positive"},
		{name: "fill missing quantity", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","price":"12.34"}`, want: "quantity is required"},
		{name: "fill null quantity", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":null,"price":"12.34"}`, want: "quantity is required"},
		{name: "fill numeric quantity", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":1,"price":"12.34"}`, want: "decimal JSON string"},
		{name: "fill zero quantity", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"0.000","price":"12.34"}`, want: "fill quantity must be non-zero"},
		{name: "mark nonzero quantity", data: `{"id":"mark-1","type":"mark","symbol":"AAPL","quantity":"1.000","price":"12.34"}`, want: "quantity is not allowed"},
		{name: "mark zero quantity", data: `{"id":"mark-1","type":"mark","symbol":"AAPL","quantity":"0.000","price":"12.34"}`, want: "quantity is not allowed"},
		{name: "mark null quantity", data: `{"id":"mark-1","type":"mark","symbol":"AAPL","quantity":null,"price":"12.34"}`, want: "quantity is not allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeEvent([]byte(test.data), bounds)
			requireErrorContains(t, err, test.want)
		})
	}
}

func TestValidateEventBounds(t *testing.T) {
	bounds := Bounds{MaxPrice: 100_00, MaxAbsQuantity: 5_000}

	t.Run("inclusive boundaries", func(t *testing.T) {
		for _, quantity := range []QtyMilli{-bounds.MaxAbsQuantity, bounds.MaxAbsQuantity} {
			event := Event{
				ID:       "fill-boundary",
				Type:     EventFill,
				Symbol:   "AAPL",
				Quantity: quantity,
				Price:    bounds.MaxPrice,
			}
			if err := ValidateEvent(event, bounds); err != nil {
				t.Errorf("ValidateEvent(quantity=%s, price=%s) returned an error: %v", quantity, event.Price, err)
			}
		}
	})

	base := Event{ID: "fill-1", Type: EventFill, Symbol: "AAPL", Quantity: 1_000, Price: 1_00}
	tests := []struct {
		name   string
		mutate func(*Event)
		want   string
	}{
		{name: "zero price", mutate: func(event *Event) { event.Price = 0 }, want: "price must be positive"},
		{name: "negative price", mutate: func(event *Event) { event.Price = -1 }, want: "price must be positive"},
		{name: "price above maximum", mutate: func(event *Event) { event.Price = bounds.MaxPrice + 1 }, want: "exceeds configured maximum"},
		{name: "zero fill quantity", mutate: func(event *Event) { event.Quantity = 0 }, want: "fill quantity must be non-zero"},
		{name: "positive quantity above maximum", mutate: func(event *Event) { event.Quantity = bounds.MaxAbsQuantity + 1 }, want: "exceeds configured maximum"},
		{name: "negative quantity above maximum", mutate: func(event *Event) { event.Quantity = -bounds.MaxAbsQuantity - 1 }, want: "exceeds configured maximum"},
		{name: "min int64 quantity", mutate: func(event *Event) { event.Quantity = QtyMilli(math.MinInt64) }, want: "absolute value overflows"},
		{name: "invalid type", mutate: func(event *Event) { event.Type = "trade" }, want: "type must be"},
		{name: "nonzero mark quantity", mutate: func(event *Event) { event.Type = EventMark }, want: "mark quantity must be zero"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := base
			test.mutate(&event)
			requireErrorContains(t, ValidateEvent(event, bounds), test.want)
		})
	}
}

func TestDecodeEventEnforcesBounds(t *testing.T) {
	bounds := Bounds{MaxPrice: 100_00, MaxAbsQuantity: 5_000}
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "positive quantity at maximum", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"5.000","price":"100.00"}`},
		{name: "negative quantity at maximum", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"-5.000","price":"100.00"}`},
		{name: "price above maximum", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"1.000","price":"100.01"}`, wantErr: "exceeds configured maximum"},
		{name: "positive quantity above maximum", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"5.001","price":"100.00"}`, wantErr: "exceeds configured maximum"},
		{name: "negative quantity above maximum", data: `{"id":"fill-1","type":"fill","symbol":"AAPL","quantity":"-5.001","price":"100.00"}`, wantErr: "exceeds configured maximum"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeEvent([]byte(test.data), bounds)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeEvent returned an error: %v", err)
				}
				return
			}
			requireErrorContains(t, err, test.wantErr)
		})
	}
}

func TestConfigValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config was rejected: %v", err)
	}

	t.Run("largest safe product used by a valid config", func(t *testing.T) {
		config := validConfig()
		config.Bounds.MaxPrice = PriceCents(math.MaxInt64 / 2)
		config.Bounds.MaxAbsQuantity = 2
		config.Limits.MaxAbsPosition = QtyLimitConfig{Warn: 1, Breach: 2}
		if err := config.Validate(); err != nil {
			t.Fatalf("config with safe max price and quantity product was rejected: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "zero max price", mutate: func(config *Config) { config.Bounds.MaxPrice = 0 }, want: "bounds.max_price must be positive"},
		{name: "negative max price", mutate: func(config *Config) { config.Bounds.MaxPrice = -1 }, want: "bounds.max_price must be positive"},
		{name: "zero max quantity", mutate: func(config *Config) { config.Bounds.MaxAbsQuantity = 0 }, want: "bounds.max_abs_quantity must be positive"},
		{name: "negative max quantity", mutate: func(config *Config) { config.Bounds.MaxAbsQuantity = -1 }, want: "bounds.max_abs_quantity must be positive"},
		{
			name: "unsafe max price times max quantity",
			mutate: func(config *Config) {
				config.Bounds.MaxPrice = PriceCents(math.MaxInt64)
				config.Bounds.MaxAbsQuantity = 2
				config.Limits.MaxAbsPosition = QtyLimitConfig{Warn: 1, Breach: 2}
			},
			want: "max_price * max_abs_quantity is unsafe",
		},
		{name: "zero position warning", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Warn = 0 }, want: "max_abs_position thresholds must be positive"},
		{name: "negative position warning", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Warn = -1 }, want: "max_abs_position thresholds must be positive"},
		{name: "zero position breach", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Breach = 0 }, want: "max_abs_position thresholds must be positive"},
		{name: "negative position breach", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Breach = -1 }, want: "max_abs_position thresholds must be positive"},
		{name: "equal position thresholds", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Warn = config.Limits.MaxAbsPosition.Breach }, want: "warn must be less than breach"},
		{name: "reversed position thresholds", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Warn = config.Limits.MaxAbsPosition.Breach + 1 }, want: "warn must be less than breach"},
		{name: "position breach above quantity bound", mutate: func(config *Config) { config.Limits.MaxAbsPosition.Breach = config.Bounds.MaxAbsQuantity + 1 }, want: "breach exceeds bounds.max_abs_quantity"},
		{name: "zero gross warning", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Warn = 0 }, want: "max_gross_notional thresholds must be positive"},
		{name: "negative gross warning", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Warn = -1 }, want: "max_gross_notional thresholds must be positive"},
		{name: "zero gross breach", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Breach = 0 }, want: "max_gross_notional thresholds must be positive"},
		{name: "negative gross breach", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Breach = -1 }, want: "max_gross_notional thresholds must be positive"},
		{name: "equal gross thresholds", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Warn = config.Limits.MaxGrossNotional.Breach }, want: "warn must be less than breach"},
		{name: "reversed gross thresholds", mutate: func(config *Config) { config.Limits.MaxGrossNotional.Warn = config.Limits.MaxGrossNotional.Breach + 1 }, want: "warn must be less than breach"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfig()
			test.mutate(&config)
			requireErrorContains(t, config.Validate(), test.want)
		})
	}
}

func TestDecodeConfigStrictJSONAndValidation(t *testing.T) {
	valid := `{
		"bounds": {
			"max_price": "100.00",
			"max_abs_quantity": "10.000"
		},
		"limits": {
			"max_abs_position": {"warn": "5.000", "breach": "8.000"},
			"max_gross_notional": {"warn": "500.00", "breach": "800.00"}
		}
	}`

	t.Run("valid", func(t *testing.T) {
		got, err := DecodeConfig(strings.NewReader(valid + "\n\t"))
		if err != nil {
			t.Fatalf("DecodeConfig returned an error: %v", err)
		}
		want := validConfig()
		if got != want {
			t.Fatalf("DecodeConfig() = %+v, want %+v", got, want)
		}
	})

	unsafeProduct := `{
		"bounds":{"max_price":"92233720368547758.07","max_abs_quantity":"0.002"},
		"limits":{
			"max_abs_position":{"warn":"0.001","breach":"0.002"},
			"max_gross_notional":{"warn":"1.00","breach":"2.00"}
		}
	}`
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "unknown top-level field", data: strings.Replace(valid, `"limits": {`, `"extra": true, "limits": {`, 1), want: `unknown field "extra"`},
		{name: "case-sensitive top-level field", data: strings.Replace(valid, `"bounds": {`, `"BOUNDS": {`, 1), want: `unknown field "BOUNDS"`},
		{name: "unknown nested bounds field", data: strings.Replace(valid, `"max_price": "100.00",`, `"max_price": "100.00", "extra": true,`, 1), want: `unknown field "extra"`},
		{name: "unknown nested limit field", data: strings.Replace(valid, `"warn": "5.000",`, `"warn": "5.000", "extra": true,`, 1), want: `unknown field "extra"`},
		{name: "duplicate nested limit field", data: strings.Replace(valid, `"warn": "5.000",`, `"warn": "5.000", "warn": "6.000",`, 1), want: `duplicate field "warn"`},
		{name: "second JSON value", data: valid + `{}`, want: "multiple JSON values"},
		{name: "trailing garbage", data: valid + ` garbage`, want: "trailing data"},
		{name: "numeric fixed-point field", data: strings.Replace(valid, `"max_price": "100.00"`, `"max_price": 100.00`, 1), want: "decimal JSON string"},
		{name: "fixed-point field with excess precision", data: strings.Replace(valid, `"max_price": "100.00"`, `"max_price": "100.001"`, 1), want: "at most 2 decimal places"},
		{name: "fixed-point field with whitespace", data: strings.Replace(valid, `"max_price": "100.00"`, `"max_price": " 100.00"`, 1), want: "surrounding whitespace"},
		{name: "missing required bound", data: strings.Replace(valid, `"max_price": "100.00",`, ``, 1), want: "bounds.max_price must be positive"},
		{name: "missing required limits", data: `{"bounds":{"max_price":"100.00","max_abs_quantity":"10.000"}}`, want: "max_abs_position thresholds must be positive"},
		{name: "unsafe bound multiplication", data: unsafeProduct, want: "max_price * max_abs_quantity is unsafe"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeConfig(strings.NewReader(test.data))
			requireErrorContains(t, err, test.want)
		})
	}
}

func assertJSONRoundTrip[T comparable](t *testing.T, value T, wantJSON string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%v) returned an error: %v", value, err)
	}
	if got := string(data); got != wantJSON {
		t.Fatalf("json.Marshal(%v) = %s, want %s", value, got, wantJSON)
	}

	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) returned an error: %v", data, err)
	}
	if decoded != value {
		t.Fatalf("JSON round trip = %v, want %v", decoded, value)
	}
}

func malformedFixedPointValues() []string {
	return []string{
		"",
		" ",
		" 1.23",
		"1.23 ",
		"\t1.23",
		"1.23\n",
		"+1.23",
		"-",
		".1",
		"1.",
		"1..2",
		"--1",
		"1e3",
		"1_000",
		"NaN",
		"one",
		"１２.３",
	}
}

func testName(index int, value string) string {
	return strings.NewReplacer(
		" ", "_space_",
		"\t", "_tab_",
		"\n", "_newline_",
		"/", "_slash_",
	).Replace(string(rune('a'+index)) + "_" + value)
}

func validConfig() Config {
	return Config{
		Bounds: Bounds{MaxPrice: 100_00, MaxAbsQuantity: 10_000},
		Limits: LimitsConfig{
			MaxAbsPosition:   QtyLimitConfig{Warn: 5_000, Breach: 8_000},
			MaxGrossNotional: MoneyLimitConfig{Warn: 500_00, Breach: 800_00},
		},
	}
}

func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
}
