package validators

import "github.com/thaolaptrinh/spatial-server/internal/validation"

type Session struct{}

func (v *Session) Name() string { return "session" }

func (v *Session) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	preDisc := findInt(baseline, validation.EvKeyDisconnectedCount)
	postDisc := findInt(postRecovery, validation.EvKeyDisconnectedCount)
	if preDisc == -1 || postDisc == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "disconnected-count evidence missing"}
	}
	if postDisc > preDisc+10 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusWarn, Detail: "disconnected sessions grew significantly"}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: "session state preserved"}
}
