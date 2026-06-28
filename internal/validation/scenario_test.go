package validation

import (
	"errors"
	"os"
	"testing"
)

type stubInfra struct {
	nodes       int
	errDB       bool
	errEndpoint bool
}

func (s *stubInfra) Endpoint(id ResourceID) (string, error) {
	if s.errEndpoint {
		return "", errors.New("no endpoint")
	}
	return "127.0.0.1:6379", nil
}
func (s *stubInfra) Processes(id ResourceID) ([]*os.Process, error) { return nil, nil }
func (s *stubInfra) Database(id ResourceID) (string, error) {
	if s.errDB {
		return "", errors.New("no database")
	}
	return "dsn://localhost", nil
}
func (s *stubInfra) RuntimeNodes() int                    { return s.nodes }
func (s *stubInfra) DialServices(ids ...ResourceID) error { return nil }
func (s *stubInfra) Close() error                         { return nil }

func TestCheckRequirements_PGUnavailable(t *testing.T) {
	met, reason := CheckRequirements(&stubInfra{errDB: true}, Requirements{PostgreSQL: true})
	if met || reason == "" {
		t.Fatal("expected unmet when PG unavailable")
	}
}

func TestCheckRequirements_RedisUnavailable(t *testing.T) {
	met, reason := CheckRequirements(&stubInfra{errEndpoint: true}, Requirements{Redis: true})
	if met || reason == "" {
		t.Fatal("expected unmet when Redis unavailable")
	}
}

func TestCheckRequirements_AllMet(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{nodes: 2},
		Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1})
	if !met {
		t.Fatal("expected met")
	}
}

func TestCheckRequirements_NotEnoughNodes(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{nodes: 0}, Requirements{MinRuntimeNodes: 2})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestCheckRequirements_ComposeRequired(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{}, Requirements{ComposeRequired: true})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestCheckRequirements_NetworkFaults(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{}, Requirements{NetworkFaults: true})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestNewSummary(t *testing.T) {
	reports := []*ValidationReport{
		{
			Scenario: ScenarioMetadata{Name: "pass"},
			Outcome:  OutcomePassed,
			Duration: 100,
			Validations: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusPass},
			},
		},
		{
			Scenario: ScenarioMetadata{Name: "fail"},
			Outcome:  OutcomeFailed,
			Duration: 200,
			Validations: []ValidationResult{
				{Status: StatusFail},
			},
		},
	}
	s := NewSummary(reports)
	if s.Total != 2 {
		t.Fatalf("expected 2 total, got %d", s.Total)
	}
	if s.Passed != 1 {
		t.Fatalf("expected 1 passed, got %d", s.Passed)
	}
	if s.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", s.Failed)
	}
	if s.Invariants.Total != 3 {
		t.Fatalf("expected 3 invariant total, got %d", s.Invariants.Total)
	}
}
