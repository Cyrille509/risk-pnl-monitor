//go:build windows

package bench

import (
	"fmt"
	"math"
	"math/bits"
	"syscall"
	"testing"
	"unsafe"
)

var (
	kernel32DLL                 = syscall.NewLazyDLL("kernel32.dll")
	queryPerformanceCounterProc = kernel32DLL.NewProc("QueryPerformanceCounter")
	queryPerformanceFreqProc    = kernel32DLL.NewProc("QueryPerformanceFrequency")
)

type latencyTimestamp int64

type latencyClock struct {
	frequency      int64
	counterAddress uintptr
}

func newLatencyClock(tb testing.TB) latencyClock {
	tb.Helper()
	if err := queryPerformanceCounterProc.Find(); err != nil {
		tb.Fatalf("resolve QueryPerformanceCounter: %v", err)
	}
	if err := queryPerformanceFreqProc.Find(); err != nil {
		tb.Fatalf("resolve QueryPerformanceFrequency: %v", err)
	}
	frequencyAddress := queryPerformanceFreqProc.Addr()
	counterAddress := queryPerformanceCounterProc.Addr()
	frequency, ok := callPerformanceCounter(frequencyAddress)
	if !ok || frequency <= 0 {
		tb.Fatalf("QueryPerformanceFrequency returned value=%d success=%t", frequency, ok)
	}
	clock := latencyClock{frequency: frequency, counterAddress: counterAddress}
	_ = clock.now() // Resolve and warm the counter call outside measurement.
	return clock
}

func (clock latencyClock) now() latencyTimestamp {
	value, ok := callPerformanceCounter(clock.counterAddress)
	if !ok {
		panic("QueryPerformanceCounter failed")
	}
	return latencyTimestamp(value)
}

func (clock latencyClock) since(start latencyTimestamp) int64 {
	end := clock.now()
	ticks := int64(end - start)
	if ticks < 0 {
		panic(fmt.Sprintf("QueryPerformanceCounter moved backwards by %d ticks", -ticks))
	}
	return scalePerformanceTicks(ticks, clock.frequency)
}

func (latencyClock) source() string {
	return "windows-query-performance-counter"
}

func (clock latencyClock) frequencyHz() int64 {
	return clock.frequency
}

func callPerformanceCounter(address uintptr) (int64, bool) {
	var value int64
	success, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(&value)))
	return value, success != 0
}

func scalePerformanceTicks(ticks, frequency int64) int64 {
	seconds := ticks / frequency
	if seconds > math.MaxInt64/1_000_000_000 {
		panic("performance-counter duration overflows nanoseconds")
	}
	remainder := uint64(ticks % frequency)
	high, low := bits.Mul64(remainder, 1_000_000_000)
	fraction, _ := bits.Div64(high, low, uint64(frequency))
	return seconds*1_000_000_000 + int64(fraction)
}

func TestScalePerformanceTicks(t *testing.T) {
	tests := []struct {
		name      string
		ticks     int64
		frequency int64
		want      int64
	}{
		{name: "zero", ticks: 0, frequency: 10_000_000, want: 0},
		{name: "one 10 MHz tick", ticks: 1, frequency: 10_000_000, want: 100},
		{name: "fractional second", ticks: 15_000_000, frequency: 10_000_000, want: 1_500_000_000},
		{name: "truncates fractional nanosecond", ticks: 1, frequency: 3, want: 333_333_333},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scalePerformanceTicks(test.ticks, test.frequency); got != test.want {
				t.Fatalf("scalePerformanceTicks(%d, %d) = %d, want %d", test.ticks, test.frequency, got, test.want)
			}
		})
	}
}
