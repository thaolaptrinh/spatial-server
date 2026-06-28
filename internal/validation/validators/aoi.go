package validators

import (
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type AOI struct{}

func (v *AOI) Name() string { return "aoi" }

func (v *AOI) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	postGhosts := findInt(postRecovery, validation.EvKeyGhostCount)
	postEntities := findInt(postRecovery, validation.EvKeyEntityCount)
	if postGhosts == -1 || postEntities == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "ghost-count or entity-count missing"}
	}
	if postEntities > 0 && postGhosts > postEntities {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail,
			Detail: fmt.Sprintf("ghosts(%d) > entities(%d) — G-02 violated", postGhosts, postEntities)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass,
		Detail: fmt.Sprintf("ghosts bounded: %d/%d", postGhosts, postEntities)}
}
