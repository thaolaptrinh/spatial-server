package validators

import (
	"fmt"
	"strconv"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type Entity struct{}

func (v *Entity) Name() string { return "entity" }

func (v *Entity) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	pre := findInt(baseline, validation.EvKeyEntityCount)
	post := findInt(postRecovery, validation.EvKeyEntityCount)
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "entity-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("entity count decreased %d→%d (I-04)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("%d→%d", pre, post)}
}

func findInt(evidence []validation.Evidence, key string) int {
	m := validation.EvidenceMap(evidence)
	if e, ok := m[key]; ok {
		v, _ := strconv.Atoi(e.Value)
		return v
	}
	return -1
}
