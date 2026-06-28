package validation

import "time"

type TickSummary struct {
	Mean  time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Max   time.Duration
	Count int
}

type RecoverySummary struct {
	Duration           time.Duration
	OwnershipRestoredAt time.Time
	FirstHealthyAt     time.Time
}

type QueueSnapshot struct {
	Min  int
	Max  int
	Mean float64
	P95  int
}

type Measurement struct {
	Tick          TickSummary
	Recovery      RecoverySummary
	Queue         map[string]QueueSnapshot
	Events        map[string]int
	Drops         map[string]int
	TickDurations []time.Duration
}
