// Package gate implements a heuristic cheap gate that decides whether a
// residual text (after the rule engine) may still contain personal data.
//
// The gate is intentionally much more sensitive than the rule engine: it does
// not need to identify the exact type or span, only whether *some* PII might
// remain. It returns a heuristic suspicion score (0..1), a routing decision,
// and the signals that fired.
package gate

// Route is the routing decision produced by the gate.
type Route string

const (
	// SAFE means the residual is unlikely to contain PII; no further analysis.
	SAFE Route = "SAFE"
	// UNCERTAIN means a cheap ML classifier should be consulted.
	UNCERTAIN Route = "UNCERTAIN"
	// LIKELY_PII means an expensive ML extractor should be run directly.
	LIKELY_PII Route = "LIKELY_PII"
)

// Signal is a single weak heuristic signal that contributed to the score.
type Signal struct {
	// Name identifies the signal (e.g. "passport_like_digits").
	Name string
	// Weight is the configured weight of the signal.
	Weight float64
	// Contribution is the actual contribution applied to the score.
	Contribution float64
}

// Decision is the result of running the gate on a residual text.
type Decision struct {
	// Score is a heuristic suspicion score in [0,1]. It is NOT a statistical
	// probability.
	Score float64
	// Route is the routing decision.
	Route Route
	// Signals lists the signals that fired and their contributions.
	Signals []Signal
}

// Config holds the gate thresholds and signal weights. Thresholds are
// configurable without changing the heuristic logic.
type Config struct {
	// LowThreshold: score < LowThreshold => SAFE.
	LowThreshold float64
	// HighThreshold: LowThreshold <= score < HighThreshold => UNCERTAIN,
	// score >= HighThreshold => LIKELY_PII.
	HighThreshold float64

	// Structural signal weights.
	PassportLikeDigits float64
	IdentifierDigits   float64
	CardLikeDigits     float64
	PhoneLikeDigits    float64
	EmailLike          float64
	DateLike           float64
	DepartmentCodeLike float64
	SecurityCodeLike   float64
	NameLike           float64

	// Contextual signal weights.
	PassportContext   float64
	SeriesNumber      float64
	Issued            float64
	DepartmentCodeCtx float64
	CitizenshipCtx    float64
	BirthCtx          float64
	AddressCtx        float64
	InnCtx            float64
	CardCtx           float64
	CvvCtx            float64
	PinCtx            float64
	DriverLicenseCtx  float64
	ClientCtx         float64

	// Public/non-PII context is a small negative signal.
	PublicContext float64
}

// DefaultConfig returns a sensible default configuration. Thresholds and
// weights are heuristic and can be tuned.
func DefaultConfig() Config {
	return Config{
		LowThreshold:  0.3,
		HighThreshold: 0.7,

		PassportLikeDigits: 0.35,
		IdentifierDigits:   0.35,
		CardLikeDigits:     0.35,
		PhoneLikeDigits:    0.35,
		EmailLike:          0.35,
		DateLike:           0.3,
		DepartmentCodeLike: 0.35,
		SecurityCodeLike:   0.35,
		NameLike:           0.35,

		PassportContext:   0.4,
		SeriesNumber:      0.3,
		Issued:            0.3,
		DepartmentCodeCtx: 0.3,
		CitizenshipCtx:    0.3,
		BirthCtx:          0.3,
		AddressCtx:        0.3,
		InnCtx:            0.3,
		CardCtx:           0.3,
		CvvCtx:            0.3,
		PinCtx:            0.3,
		DriverLicenseCtx:  0.3,
		ClientCtx:         0.25,

		PublicContext: 0.1,
	}
}

// RouteForScore maps a score to a route using the config thresholds.
func (c Config) RouteForScore(score float64) Route {
	if score < c.LowThreshold {
		return SAFE
	}
	if score < c.HighThreshold {
		return UNCERTAIN
	}
	return LIKELY_PII
}
