package runtime

import "runtime"

// runtimeMemStats is aliased to keep harness.go decoupled from runtime details.
type runtimeMemStats = runtime.MemStats

func snapshotMem(m *runtimeMemStats) { runtime.ReadMemStats(m) }

func goroutineCount() int { return runtime.NumGoroutine() }
