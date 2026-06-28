package validators

import "github.com/thaolaptrinh/spatial-server/internal/validation"

type Scheduler struct{}

func (v *Scheduler) Name() string { return "scheduler" }

func (v *Scheduler) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	preDrops := findInt(baseline, validation.EvKeyCmdDrops)
	postDrops := findInt(postRecovery, validation.EvKeyCmdDrops)
	if preDrops == -1 || postDrops == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "cmd-drops evidence missing"}
	}
	if postDrops > preDrops {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusWarn, Detail: "command drops observed (T-04)"}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: "no command drops"}
}
