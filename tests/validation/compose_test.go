//go:build validation

package validation

import (
	"testing"

	v "github.com/thaolaptrinh/spatial-server/internal/validation"
)

func TestComposeChaosScenarios(t *testing.T) {
	for _, sc := range ComposeScenarios() {
		t.Run(sc.Metadata.ID, func(t *testing.T) {
			t.Skip("compose scenarios require Docker Compose orchestration")
			_ = sc
		})
		_ = sc.Metadata
	}
	_ = v.ModeCompose
}
