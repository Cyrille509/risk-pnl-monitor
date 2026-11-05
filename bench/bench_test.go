package bench

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"runtime"
	"sort"
	"testing"

	riskmon "riskmon/internal"
)

const (
	benchmarkCorpusEvents   = 128
	benchmarkNormalCycles   = 7
	benchmarkWarmupCorpora  = 1_024
	benchmarkWarmupEvents   = benchmarkWarmupCorpora * benchmarkCorpusEvents
	benchmarkAuditBufferLen = 64 * 1_024

	benchmarkAlertsPerCorpus     = 15
	benchmarkAuditRecords        = benchmarkCorpusEvents + benchmarkAlertsPerCorpus
	benchmarkAuditBytesPerCorpus = 49_260
	benchmarkAuditSHA256         = "0b3fdafac948626dbcfa664531a736d8640ac1e95cbd5aee2b6b93e6709fb926"
)

var (
	benchmarkApplyResultSink riskmon.ApplyResult
	benchmarkAuditBytesSink  int64
	benchmarkClockSink       int64
)

// Samples cover decoding through buffered audit writes, excluding the final flush.
// Input and output are in memory. Setup and validation are outside the timer.
// Each operation is 128 events; ns/op is per corpus, percentiles are per event.
func BenchmarkEndToEndServiceLatency(b *testing.B) {
	b.StopTimer()
	corpus := newBenchmarkCorpus(b)
	warmFullPath(b, corpus)

	totalEvents := measuredEventCount(b, b.N)
	samples := make([]int64, totalEvents)
	engine := newBenchmarkEngine(b)
	decoder := riskmon.NewEventDecoder(newRepeatingReader(corpus.jsonl), benchmarkConfig().Bounds)
	auditSink := &countingDiscard{}
	auditBuffer := bufio.NewWriterSize(auditSink, benchmarkAuditBufferLen)
	auditWriter := riskmon.NewAuditWriter(auditBuffer)
	clock := newLatencyClock(b)
	runtime.GC()

	b.ReportAllocs()
	b.SetBytes(int64(len(corpus.jsonl)))
	b.ResetTimer()
	b.StartTimer()
	sampleIndex := 0
	var lastResult riskmon.ApplyResult
	for range b.N {
		for range benchmarkCorpusEvents {
			started := clock.now()
			event, err := decoder.Next()
			if err != nil {
				b.Fatalf("decode measured event %d: %v", sampleIndex, err)
			}
			result, err := engine.Apply(event)
			if err != nil {
				b.Fatalf("apply measured event %d: %v", sampleIndex, err)
			}
			if err := auditWriter.WriteResult(result); err != nil {
				b.Fatalf("audit measured event %d: %v", sampleIndex, err)
			}
			samples[sampleIndex] = clock.since(started)
			sampleIndex++
			lastResult = result
		}
	}
	b.StopTimer()

	if err := auditBuffer.Flush(); err != nil {
		b.Fatalf("flush measured audit buffer: %v", err)
	}
	validateCompletedRun(b, engine, auditSink.bytes, b.N)
	benchmarkApplyResultSink = lastResult
	benchmarkAuditBytesSink = auditSink.bytes
	reportLatencyMetrics(b, samples, "event")
}

// Same path as the latency benchmark, without per-event clock reads.
// ns/op is per 128-event corpus; events/s is per event.
func BenchmarkReplayFullPathThroughput(b *testing.B) {
	b.StopTimer()
	corpus := newBenchmarkCorpus(b)
	warmFullPath(b, corpus)

	totalEvents := measuredEventCount(b, b.N)
	engine := newBenchmarkEngine(b)
	decoder := riskmon.NewEventDecoder(newRepeatingReader(corpus.jsonl), benchmarkConfig().Bounds)
	auditSink := &countingDiscard{}
	auditBuffer := bufio.NewWriterSize(auditSink, benchmarkAuditBufferLen)
	auditWriter := riskmon.NewAuditWriter(auditBuffer)
	clock := newLatencyClock(b)
	runtime.GC()

	b.ReportAllocs()
	b.SetBytes(int64(len(corpus.jsonl)))
	b.ResetTimer()
	b.StartTimer()
	throughputStarted := clock.now()
	var lastResult riskmon.ApplyResult
	eventIndex := 0
	for range b.N {
		for range benchmarkCorpusEvents {
			event, err := decoder.Next()
			if err != nil {
				b.Fatalf("decode measured event %d: %v", eventIndex, err)
			}
			result, err := engine.Apply(event)
			if err != nil {
				b.Fatalf("apply measured event %d: %v", eventIndex, err)
			}
			if err := auditWriter.WriteResult(result); err != nil {
				b.Fatalf("audit measured event %d: %v", eventIndex, err)
			}
			lastResult = result
			eventIndex++
		}
	}
	throughputElapsed := clock.since(throughputStarted)
	b.StopTimer()

	if err := auditBuffer.Flush(); err != nil {
		b.Fatalf("flush measured audit buffer: %v", err)
	}
	validateCompletedRun(b, engine, auditSink.bytes, b.N)
	benchmarkApplyResultSink = lastResult
	benchmarkAuditBytesSink = auditSink.bytes
	if throughputElapsed <= 0 {
		b.Fatalf("non-positive throughput interval %d ns", throughputElapsed)
	}
	eventsPerSecond := float64(totalEvents) * 1e9 / float64(throughputElapsed)
	b.ReportMetric(eventsPerSecond, "events/s")
	b.Logf("RESULT throughput events=%d elapsed_ns=%d events_per_sec=%.9f",
		totalEvents, throughputElapsed, eventsPerSecond)
}

// Clock overhead is measured separately, not subtracted from event latency.
func BenchmarkClockPairLatency(b *testing.B) {
	b.StopTimer()
	clock := newLatencyClock(b)
	for range benchmarkWarmupEvents {
		started := clock.now()
		benchmarkClockSink = clock.since(started)
	}
	samples := make([]int64, b.N)
	runtime.GC()

	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for index := range b.N {
		started := clock.now()
		samples[index] = clock.since(started)
	}
	b.StopTimer()

	if len(samples) > 0 {
		benchmarkClockSink = samples[len(samples)-1]
	}
	reportLatencyMetrics(b, samples, "pair")
	b.Logf("RESULT clock source=%s frequency_hz=%d overhead_subtracted=false",
		clock.source(), clock.frequencyHz())
}

func TestBenchmarkCorpusGolden(t *testing.T) {
	corpus := newBenchmarkCorpus(t)
	if len(corpus.events) != benchmarkCorpusEvents {
		t.Fatalf("corpus has %d events, want %d", len(corpus.events), benchmarkCorpusEvents)
	}

	firstSummary, firstAudit := replayOneCorpus(t, corpus)
	validateSummary(t, firstSummary, 1)
	if records := bytes.Count(firstAudit, []byte{'\n'}); records != benchmarkAuditRecords {
		t.Fatalf("audit has %d records, want %d", records, benchmarkAuditRecords)
	}
	if len(firstAudit) != benchmarkAuditBytesPerCorpus {
		t.Fatalf("audit has %d bytes, want %d", len(firstAudit), benchmarkAuditBytesPerCorpus)
	}
	firstHash := fmt.Sprintf("%x", sha256.Sum256(firstAudit))
	if firstHash != benchmarkAuditSHA256 {
		t.Fatalf("audit SHA-256 = %s, want %s", firstHash, benchmarkAuditSHA256)
	}

	secondSummary, secondAudit := replayOneCorpus(t, corpus)
	if !reflect.DeepEqual(secondSummary, firstSummary) {
		t.Fatalf("second summary differs\nfirst:  %+v\nsecond: %+v", firstSummary, secondSummary)
	}
	if !bytes.Equal(secondAudit, firstAudit) {
		t.Fatal("second audit output differs from first output")
	}
}

func TestCalculateLatencyMetricsUsesNearestRank(t *testing.T) {
	samples := make([]int64, 1_000)
	for index := range samples {
		samples[index] = int64(1_000 - index)
	}
	got := calculateLatencyMetrics(samples)
	want := latencyMetrics{
		p50:   500,
		p95:   950,
		p99:   990,
		p999:  999,
		total: 500_500,
		mean:  500.5,
		max:   1_000,
		count: 1_000,
	}
	if got != want {
		t.Fatalf("latency metrics = %+v, want %+v", got, want)
	}
}

type benchmarkCorpus struct {
	events []riskmon.Event
	jsonl  []byte
}

func newBenchmarkCorpus(tb testing.TB) benchmarkCorpus {
	tb.Helper()
	events := make([]riskmon.Event, 0, benchmarkCorpusEvents)
	symbols := [...]string{"AAPL", "MSFT", "NVDA", "TSLA"}
	for cycle := range benchmarkNormalCycles {
		for _, symbol := range symbols {
			events = append(events, benchmarkMark(
				fmt.Sprintf("normal-%d-open-mark-%s", cycle, symbol), symbol, 100_00,
			))
		}
		for _, symbol := range symbols {
			events = append(events, benchmarkFill(
				fmt.Sprintf("normal-%d-buy-%s", cycle, symbol), symbol, 2_000, 100_00,
			))
		}
		for _, symbol := range symbols {
			events = append(events, benchmarkMark(
				fmt.Sprintf("normal-%d-revalue-%s", cycle, symbol), symbol, 105_00,
			))
		}
		for _, symbol := range symbols {
			events = append(events, benchmarkFill(
				fmt.Sprintf("normal-%d-close-%s", cycle, symbol), symbol, -2_000, 100_00,
			))
		}
	}

	events = append(events,
		benchmarkMark("stress-01", "AAPL", 100_00),
		benchmarkFill("stress-02", "AAPL", 5_000, 100_00),
		benchmarkFill("stress-03", "AAPL", 5_000, 110_00),
		benchmarkMark("stress-04", "AAPL", 190_00),
		benchmarkFill("stress-05", "AAPL", 3_000, 120_00),
		benchmarkMark("stress-06", "AAPL", 200_00),
		benchmarkFill("stress-07", "AAPL", -4_000, 130_00),
		benchmarkFill("stress-08", "AAPL", -6_000, 90_00),
		benchmarkFill("stress-09", "AAPL", -5_000, 95_00),
		benchmarkFill("stress-10", "AAPL", -6_000, 90_00),
		benchmarkFill("stress-11", "AAPL", -5_000, 85_00),
		benchmarkMark("stress-12", "AAPL", 180_00),
		benchmarkFill("stress-13", "AAPL", 5_000, 80_00),
		benchmarkFill("stress-14", "AAPL", 6_000, 100_00),
		benchmarkFill("stress-15", "AAPL", 4_000, 90_00),
		benchmarkFill("stress-16", "AAPL", -2_000, 135_03),
	)
	if len(events) != benchmarkCorpusEvents {
		tb.Fatalf("constructed %d corpus events, want %d", len(events), benchmarkCorpusEvents)
	}

	var jsonl bytes.Buffer
	jsonl.Grow(len(events) * 96)
	encoder := json.NewEncoder(&jsonl)
	encoder.SetEscapeHTML(false)
	for index, event := range events {
		if err := encoder.Encode(event); err != nil {
			tb.Fatalf("encode corpus event %d: %v", index, err)
		}
	}
	return benchmarkCorpus{events: events, jsonl: jsonl.Bytes()}
}

func benchmarkMark(id, symbol string, price riskmon.PriceCents) riskmon.Event {
	return riskmon.Event{ID: id, Type: riskmon.EventMark, Symbol: symbol, Price: price}
}

func benchmarkFill(id, symbol string, quantity riskmon.QtyMilli, price riskmon.PriceCents) riskmon.Event {
	return riskmon.Event{
		ID:       id,
		Type:     riskmon.EventFill,
		Symbol:   symbol,
		Quantity: quantity,
		Price:    price,
	}
}

func benchmarkConfig() riskmon.Config {
	return riskmon.Config{
		Bounds: riskmon.Bounds{
			MaxPrice:       100_000_000,
			MaxAbsQuantity: 10_000_000_000,
		},
		Limits: riskmon.LimitsConfig{
			MaxAbsPosition: riskmon.QtyLimitConfig{
				Warn:   8_000,
				Breach: 12_000,
			},
			MaxGrossNotional: riskmon.MoneyLimitConfig{
				Warn:   180_000,
				Breach: 250_000,
			},
		},
	}
}

func newBenchmarkEngine(tb testing.TB) *riskmon.Engine {
	tb.Helper()
	engine, err := riskmon.NewEngine(benchmarkConfig())
	if err != nil {
		tb.Fatalf("create benchmark engine: %v", err)
	}
	return engine
}

func warmFullPath(tb testing.TB, corpus benchmarkCorpus) {
	tb.Helper()
	engine := newBenchmarkEngine(tb)
	decoder := riskmon.NewEventDecoder(newRepeatingReader(corpus.jsonl), benchmarkConfig().Bounds)
	auditSink := &countingDiscard{}
	auditBuffer := bufio.NewWriterSize(auditSink, benchmarkAuditBufferLen)
	auditWriter := riskmon.NewAuditWriter(auditBuffer)
	eventIndex := 0
	for range benchmarkWarmupCorpora {
		for range benchmarkCorpusEvents {
			event, err := decoder.Next()
			if err != nil {
				tb.Fatalf("decode warmup event %d: %v", eventIndex, err)
			}
			result, err := engine.Apply(event)
			if err != nil {
				tb.Fatalf("apply warmup event %d: %v", eventIndex, err)
			}
			if err := auditWriter.WriteResult(result); err != nil {
				tb.Fatalf("audit warmup event %d: %v", eventIndex, err)
			}
			eventIndex++
		}
	}
	if eventIndex != benchmarkWarmupEvents {
		tb.Fatalf("processed %d warmup events, want %d", eventIndex, benchmarkWarmupEvents)
	}
	if err := auditBuffer.Flush(); err != nil {
		tb.Fatalf("flush warmup audit buffer: %v", err)
	}
	validateCompletedRun(tb, engine, auditSink.bytes, benchmarkWarmupCorpora)
}

func replayOneCorpus(tb testing.TB, corpus benchmarkCorpus) (riskmon.Summary, []byte) {
	tb.Helper()
	var audit bytes.Buffer
	result, err := riskmon.RunReplay(bytes.NewReader(corpus.jsonl), &audit, benchmarkConfig())
	if err != nil {
		tb.Fatalf("replay benchmark corpus: %v", err)
	}
	return result.Summary, audit.Bytes()
}

func validateCompletedRun(tb testing.TB, engine *riskmon.Engine, auditBytes int64, corpusCount int) {
	tb.Helper()
	summary, err := engine.Summary()
	if err != nil {
		tb.Fatalf("summarize benchmark run: %v", err)
	}
	validateSummary(tb, summary, corpusCount)
	if auditBytes >= 0 {
		if int64(corpusCount) > math.MaxInt64/int64(benchmarkAuditBytesPerCorpus) {
			tb.Fatalf("corpus count %d overflows expected audit bytes", corpusCount)
		}
		wantAuditBytes := int64(benchmarkAuditBytesPerCorpus) * int64(corpusCount)
		if auditBytes != wantAuditBytes {
			tb.Fatalf("audit bytes = %d, want %d", auditBytes, wantAuditBytes)
		}
	}
}

func validateSummary(tb testing.TB, got riskmon.Summary, corpusCount int) {
	tb.Helper()
	if corpusCount < 0 || corpusCount > math.MaxInt/benchmarkAlertsPerCorpus ||
		int64(corpusCount) > math.MaxInt64/benchmarkCorpusEvents {
		tb.Fatalf("invalid corpus count %d", corpusCount)
	}
	mark180 := riskmon.PriceCents(180_00)
	mark105MSFT := riskmon.PriceCents(105_00)
	mark105NVDA := riskmon.PriceCents(105_00)
	mark105TSLA := riskmon.PriceCents(105_00)
	want := riskmon.Summary{
		ProcessedEvents: int64(corpusCount) * benchmarkCorpusEvents,
		Positions: []riskmon.Position{
			{Symbol: "AAPL", LastMark: &mark180},
			{Symbol: "MSFT", LastMark: &mark105MSFT},
			{Symbol: "NVDA", LastMark: &mark105NVDA},
			{Symbol: "TSLA", LastMark: &mark105TSLA},
		},
		AlertsEmitted: corpusCount * benchmarkAlertsPerCorpus,
	}
	if !reflect.DeepEqual(got, want) {
		tb.Fatalf("benchmark summary mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func measuredEventCount(tb testing.TB, corpusCount int) int {
	tb.Helper()
	if corpusCount < 0 || corpusCount > math.MaxInt/benchmarkCorpusEvents {
		tb.Fatalf("corpus count %d overflows event count", corpusCount)
	}
	return corpusCount * benchmarkCorpusEvents
}

type repeatingReader struct {
	data   []byte
	offset int
}

func newRepeatingReader(data []byte) *repeatingReader {
	return &repeatingReader{data: data}
}

func (reader *repeatingReader) Read(destination []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, io.EOF
	}
	written := 0
	for written < len(destination) {
		copied := copy(destination[written:], reader.data[reader.offset:])
		written += copied
		reader.offset += copied
		if reader.offset == len(reader.data) {
			reader.offset = 0
		}
	}
	return written, nil
}

type countingDiscard struct {
	bytes int64
}

func (writer *countingDiscard) Write(data []byte) (int, error) {
	writer.bytes += int64(len(data))
	return len(data), nil
}

type latencyMetrics struct {
	p50   int64
	p95   int64
	p99   int64
	p999  int64
	total uint64
	mean  float64
	max   int64
	count int
}

func reportLatencyMetrics(b *testing.B, samples []int64, sampleUnit string) {
	b.Helper()
	metrics := calculateLatencyMetrics(samples)
	b.ReportMetric(float64(metrics.p50), "p50-ns/"+sampleUnit)
	b.ReportMetric(float64(metrics.p95), "p95-ns/"+sampleUnit)
	b.ReportMetric(float64(metrics.p99), "p99-ns/"+sampleUnit)
	if metrics.count >= 1_000 {
		b.ReportMetric(float64(metrics.p999), "p99.9-ns/"+sampleUnit)
		b.Logf("RESULT latency benchmark=%s unit=%s samples=%d p50_ns=%d p95_ns=%d p99_ns=%d p999_ns=%d sum_ns=%d mean_ns=%.9f max_ns=%d",
			b.Name(), sampleUnit, metrics.count, metrics.p50, metrics.p95, metrics.p99, metrics.p999,
			metrics.total, metrics.mean, metrics.max)
	} else {
		b.Logf("RESULT latency benchmark=%s unit=%s samples=%d p50_ns=%d p95_ns=%d p99_ns=%d p999_ns=unsupported sum_ns=%d mean_ns=%.9f max_ns=%d",
			b.Name(), sampleUnit, metrics.count, metrics.p50, metrics.p95, metrics.p99,
			metrics.total, metrics.mean, metrics.max)
	}
	b.ReportMetric(metrics.mean, "mean-ns/"+sampleUnit)
	b.ReportMetric(float64(metrics.max), "max-ns/"+sampleUnit)
	b.ReportMetric(float64(metrics.count), "samples")
}

func calculateLatencyMetrics(samples []int64) latencyMetrics {
	if len(samples) == 0 {
		return latencyMetrics{}
	}
	sort.Slice(samples, func(left, right int) bool { return samples[left] < samples[right] })
	var total uint64
	for _, sample := range samples {
		if sample < 0 {
			panic(fmt.Sprintf("negative latency sample %d", sample))
		}
		total += uint64(sample)
	}
	return latencyMetrics{
		p50:   nearestRank(samples, 50, 100),
		p95:   nearestRank(samples, 95, 100),
		p99:   nearestRank(samples, 99, 100),
		p999:  nearestRank(samples, 999, 1_000),
		total: total,
		mean:  float64(total) / float64(len(samples)),
		max:   samples[len(samples)-1],
		count: len(samples),
	}
}

func nearestRank(sortedSamples []int64, numerator, denominator int) int64 {
	if len(sortedSamples) == 0 || numerator <= 0 || denominator <= 0 || numerator > denominator {
		panic("invalid nearest-rank percentile inputs")
	}
	sampleCount := len(sortedSamples)
	rank := (sampleCount/denominator)*numerator +
		((sampleCount%denominator)*numerator+denominator-1)/denominator
	return sortedSamples[rank-1]
}
