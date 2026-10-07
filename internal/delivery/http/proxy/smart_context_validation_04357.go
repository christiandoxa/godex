package proxy

type smartContextValidationStats struct {
	rehydratedRefs          int
	toolOutputsCondensed    int
	duplicateTexts          int
	crossTurnDuplicateTexts int
	repeatToolOutputRefs    int
	staticContextDeltas     int
}

func smartContextValidationReason(
	criticalSignalLoss bool,
	fallbackExact bool,
	reasonBits uint64,
	stats smartContextValidationStats,
) string {
	if criticalSignalLoss {
		return "critical_signal_loss"
	}
	rehydrateOnly := stats.rehydratedRefs > 0 &&
		stats.toolOutputsCondensed == 0 &&
		stats.duplicateTexts == 0 &&
		stats.crossTurnDuplicateTexts == 0 &&
		stats.repeatToolOutputRefs == 0 &&
		stats.staticContextDeltas == 0
	if rehydrateOnly || !fallbackExact {
		return ""
	}
	switch {
	case reasonBits&(1<<4) != 0:
		return "critical_signal_loss"
	case reasonBits&(1<<5) != 0:
		return "missing_rehydrate_refs"
	case reasonBits&(1<<0) != 0:
		return "exactness_required"
	case reasonBits&(1<<6) != 0:
		return "empty_after_payload"
	case reasonBits&(1<<3) != 0:
		return "token_savings_below_safety_margin"
	case reasonBits&(1<<1) != 0:
		return "unsupported_tokenizer"
	default:
		return "token_budget_did_not_improve"
	}
}
