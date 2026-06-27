package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Report struct {
	Scenario string  `json:"scenario"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	P50      float64 `json:"p50_us"`
	P95      float64 `json:"p95_us"`
	P99      float64 `json:"p99_us"`
	P999     float64 `json:"p999_us"`
	Packets  int     `json:"packets"`
	Pass     bool    `json:"pass"`
}

func NewReport(scenario string) *Report {
	return &Report{Scenario: scenario, Start: time.Now().Format(time.RFC3339)}
}

func (r *Report) Write() error {
	r.End = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fmt.Sprintf("benchmarks/reports/%s-%s.json", r.Scenario, time.Now().Format("20060102T150405")), data, 0644)
}

func (r *Report) PrintSummary() {
	fmt.Printf("\n=== %s ===\nP50: %.0fus  P95: %.0fus  P99: %.0fus  Packets: %d  Pass: %t\n",
		r.Scenario, r.P50, r.P95, r.P99, r.Packets, r.Pass)
}

func (r *Report) AssertP95(h *Histogram, budgetUs float64) error {
	p95 := h.Percentile(95)
	if p95 > budgetUs {
		return fmt.Errorf("p95 %.2fus exceeds budget %.2fus", p95, budgetUs)
	}
	r.P95 = p95
	return nil
}
