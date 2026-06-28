package validation

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const frameworkVersion = "1.0.0"

type FrameworkMeta struct {
	FrameworkVersion string
	ScenarioVersion  string
	ExecTimestamp    time.Time
}

type Outcome string

const (
	OutcomePassed   Outcome = "passed"
	OutcomeFailed   Outcome = "failed"
	OutcomeSkipped  Outcome = "skipped"
	OutcomeTimedOut Outcome = "timed_out"
	OutcomeError    Outcome = "error"
)

type ValidationReport struct {
	Framework    FrameworkMeta
	Scenario     ScenarioMetadata
	Outcome      Outcome
	Duration     time.Duration
	Baseline     []Evidence
	PostRecovery []Evidence
	Validations  []ValidationResult
	Measurement  Measurement
	RootCause    string
}

func (r *ValidationReport) CombinedEvidence() []Evidence {
	n := len(r.Baseline) + len(r.PostRecovery)
	combined := make([]Evidence, 0, n)
	combined = append(combined, r.Baseline...)
	combined = append(combined, r.PostRecovery...)
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Timestamp.Before(combined[j].Timestamp)
	})
	return combined
}

type SummaryReport struct {
	Framework     FrameworkMeta
	ExecTimestamp time.Time
	Total         int
	Passed        int
	Failed        int
	Skipped       int
	TimedOut      int
	Errors        int
	TotalDuration time.Duration
	Recovery      struct {
		Count        int
		MinDuration  time.Duration
		MaxDuration  time.Duration
		MeanDuration time.Duration
	}
	Invariants struct {
		Total   int
		Passed  int
		Warned  int
		Failed  int
		Skipped int
	}
	Scenarios []SummaryEntry
}

type SummaryEntry struct {
	Name     string
	Outcome  Outcome
	Duration time.Duration
	Reason   string
}

func NewSummary(reports []*ValidationReport) *SummaryReport {
	s := &SummaryReport{
		Framework: FrameworkMeta{
			FrameworkVersion: frameworkVersion,
			ScenarioVersion:  "",
			ExecTimestamp:    time.Now(),
		},
	}
	var recoveryDurations []time.Duration
	for _, r := range reports {
		s.Total++
		entry := SummaryEntry{
			Name:     r.Scenario.Name,
			Outcome:  r.Outcome,
			Duration: r.Duration,
			Reason:   r.RootCause,
		}
		s.Scenarios = append(s.Scenarios, entry)
		s.TotalDuration += r.Duration
		switch r.Outcome {
		case OutcomePassed:
			s.Passed++
		case OutcomeFailed:
			s.Failed++
		case OutcomeSkipped:
			s.Skipped++
		case OutcomeTimedOut:
			s.TimedOut++
		case OutcomeError:
			s.Errors++
		}
		for _, v := range r.Validations {
			s.Invariants.Total++
			switch v.Status {
			case StatusPass:
				s.Invariants.Passed++
			case StatusWarn:
				s.Invariants.Warned++
			case StatusFail:
				s.Invariants.Failed++
			case StatusSkip:
				s.Invariants.Skipped++
			}
		}
		if r.Measurement.Recovery.Duration > 0 {
			recoveryDurations = append(recoveryDurations, r.Measurement.Recovery.Duration)
		}
	}
	if n := len(recoveryDurations); n > 0 {
		s.Recovery.Count = n
		s.Recovery.MinDuration = recoveryDurations[0]
		s.Recovery.MaxDuration = recoveryDurations[0]
		var total time.Duration
		for _, d := range recoveryDurations {
			total += d
			if d < s.Recovery.MinDuration {
				s.Recovery.MinDuration = d
			}
			if d > s.Recovery.MaxDuration {
				s.Recovery.MaxDuration = d
			}
		}
		s.Recovery.MeanDuration = total / time.Duration(n)
	}
	return s
}

type MarkdownReporter struct{}

func (m *MarkdownReporter) Generate(report *ValidationReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chaos Report: %s\n\n", report.Scenario.Name)
	fmt.Fprintf(&b, "| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&b, "| Outcome | %s |\n", report.Outcome)
	fmt.Fprintf(&b, "| Duration | %s |\n", report.Duration)
	fmt.Fprintf(&b, "| Expected | %s |\n\n", report.Scenario.ExpectedBehavior)
	fmt.Fprintf(&b, "## Validations\n\n| Validator | Status | Detail |\n|-----------|--------|--------|\n")
	for _, v := range report.Validations {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", v.Validator, v.Status, v.Detail)
	}
	_, _ = os.Stdout.Write([]byte(b.String()))
	return nil
}

func MarkdownReportString(report *ValidationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chaos Report: %s\n\n", report.Scenario.Name)
	fmt.Fprintf(&b, "| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&b, "| Outcome | %s |\n", report.Outcome)
	fmt.Fprintf(&b, "| Duration | %s |\n", report.Duration)
	fmt.Fprintf(&b, "| Expected | %s |\n\n", report.Scenario.ExpectedBehavior)
	fmt.Fprintf(&b, "## Validations\n\n| Validator | Status | Detail |\n|-----------|--------|--------|\n")
	for _, v := range report.Validations {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", v.Validator, v.Status, v.Detail)
	}
	return b.String()
}
