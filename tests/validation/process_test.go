//go:build validation

package validation

import (
	"context"
	"testing"

	v "github.com/thaolaptrinh/spatial-server/internal/validation"
)

func TestProcessChaosScenarios(t *testing.T) {
	scenarios := ProcessScenarios(t)
	if err := v.ValidateScenarios(scenarios); err != nil {
		t.Fatalf("invalid scenarios: %v", err)
	}
	for _, sc := range scenarios {
		t.Run(sc.Metadata.ID, func(t *testing.T) {
			report := v.NewRunner().Run(context.Background(), sc)
			if report.Outcome == v.OutcomeSkipped {
				t.Skipf("skipped: %s", report.RootCause)
			}
			if report.Outcome != v.OutcomePassed {
				t.Errorf("scenario %s: %s — %s", sc.Metadata.Name, report.Outcome, report.RootCause)
				t.Log(v.MarkdownReportString(report))
			}
		})
	}
}
