# Risk and PnL monitor

A small Go program that reads trades and price updates from a file, tracks positions and PnL, and flags risk limits. It runs offline on recorded events.

## Run it

Requires Go 1.24 or newer.

```sh
go run ./cmd/riskmon
```

This replays `testdata/events_day.jsonl` using the limits in `config.json`, prints a summary, and writes `audit.jsonl`. The audit file is overwritten each run.

To use your own files:

```sh
go run ./cmd/riskmon -config config.json -events events.jsonl -audit audit.jsonl
```

Events are one JSON object per line. A fill changes the position; a mark updates its market value.

```jsonl
{"id":"evt-001","type":"fill","symbol":"AAPL","quantity":"5.000","price":"100.00"}
{"id":"evt-002","type":"mark","symbol":"AAPL","price":"108.00"}
```

Use negative quantities for sells. Prices and quantities are strings, with up to two and three decimal places respectively.

## Data flow

```text
events.jsonl -> decode and validate -> positions and PnL -> risk checks
                                                        -> audit log + summary
```

Events are processed in file order. Invalid input stops the replay, leaving the completed events in the audit log.

## Design notes

- Prices and quantities use integers to avoid floating-point drift. Division truncates toward zero, so rounding still matters.
- PnL uses average cost. Closing part of a position keeps its average cost; flipping sides opens the remainder at the new fill price.
- Updates run on one thread to keep event ordering straightforward.
- Risk alerts fire when a limit changes state, so the same breach does not produce an alert on every update. Limits warn; they do not reject trades.
- The audit has stable ordering and no timestamps, making replay results easy to compare in tests.

An unmarked position counts as zero exposure until its first price update. There is no live feed or duplicate-event detection yet.

## Checks

```sh
go test ./...
go vet ./...
```

Tests cover position changes, PnL, limit transitions, invalid input, and repeatable replay output. Performance benchmarks are in [bench/](bench/bench_test.go).
