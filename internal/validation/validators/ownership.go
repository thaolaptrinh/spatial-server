package validators

import (
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type Ownership struct{}

func (v *Ownership) Name() string { return "ownership" }

func (v *Ownership) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	pre := findInt(baseline, validation.EvKeyZoneOwnerCount)
	post := findInt(postRecovery, validation.EvKeyZoneOwnerCount)
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "zone-owner-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("zone owners lost: %d→%d (O-01)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("ownership preserved: %d→%d", pre, post)}
}
