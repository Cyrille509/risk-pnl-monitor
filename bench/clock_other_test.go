//go:build !windows

package bench

import (
	"testing"
	"time"
)

type latencyTimestamp struct {
	value time.Time
}

type latencyClock struct{}

func newLatencyClock(testing.TB) latencyClock {
	return latencyClock{}
}

func (latencyClock) now() latencyTimestamp {
	return latencyTimestamp{value: time.Now()}
}

func (latencyClock) since(start latencyTimestamp) int64 {
	return time.Since(start.value).Nanoseconds()
}

func (latencyClock) source() string {
	return "go-monotonic-time"
}

func (latencyClock) frequencyHz() int64 {
	return 0
}
