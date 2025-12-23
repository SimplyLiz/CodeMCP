package envelope

// ScoreToTier converts a completeness score (0.0-1.0) to a confidence tier.
//
// Tier mapping:
//   - 0.95+ -> high (SCIP, fresh index)
//   - 0.70-0.94 -> medium (LSP or stale SCIP)
//   - 0.30-0.69 -> low (heuristic/git-only)
//   - <0.30 -> speculative (cross-repo, uncommitted)
func ScoreToTier(score float64) ConfidenceTier {
	switch {
	case score >= 0.95:
		return TierHigh
	case score >= 0.70:
		return TierMedium
	case score >= 0.30:
		return TierLow
	default:
		return TierSpeculative
	}
}

// TierFromContext determines the appropriate tier based on available context.
func TierFromContext(hasSCIP, isSCIPFresh, isCrossRepo bool) ConfidenceTier {
	if isCrossRepo {
		return TierSpeculative
	}
	if hasSCIP && isSCIPFresh {
		return TierHigh
	}
	if hasSCIP {
		return TierMedium
	}
	return TierLow
}
