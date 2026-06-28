package framework

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// CaptureCPU starts a CPU profile written to path; the returned func stops and
// flushes it. It is a no-op (returns a no-op stop) if path is empty.
func CaptureCPU(path string) (stop func()) {
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		return func() {}
	}
	_ = pprof.StartCPUProfile(f)
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
}

// WriteHeap writes a heap profile to path.
func WriteHeap(path string) {
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	runtime.GC()
	_ = pprof.WriteHeapProfile(f)
}

// WriteProfile writes a named profile (e.g. "mutex", "block", "goroutine") to path.
func WriteProfile(name, path string) {
	if path == "" {
		return
	}
	p := pprof.Lookup(name)
	if p == nil {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = p.WriteTo(f, 0)
}

// EnableMutexProfile turns on mutex profiling (rate 0 disables).
func EnableMutexProfile(rate int) {
	runtime.SetMutexProfileFraction(rate)
}

// EnableBlockProfile turns on block profiling (rate 0 disables).
func EnableBlockProfile(rate int) {
	runtime.SetBlockProfileRate(rate)
}

// CaptureTrace starts an execution trace written to path; the returned func
// stops and flushes it.
func CaptureTrace(path string) (stop func()) {
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		return func() {}
	}
	if err := trace.Start(f); err != nil {
		_ = f.Close()
		return func() {}
	}
	return func() {
		trace.Stop()
		_ = f.Close()
	}
}
