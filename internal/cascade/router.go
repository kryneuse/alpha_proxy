// Package cascade implements the routing cascade:
//
//	chunk -> Rule Engine -> residual -> Router -> small gate -> big model
//
// The router decides whether a residual text (after the rule engine masked its
// spans) still needs further analysis. It only distinguishes two cases:
//
//	DONE        residual is empty, whitespace, or only allowed delimiters
//	CHECK_SMALL residual contains any other content
//
// The old heuristic gate (SAFE / UNCERTAIN / LIKELY_PII by score) is not part
// of the working cascade. Absence of familiar words or numeric patterns does
// not prove the absence of personal data.
package cascade

import (
	"unicode"
)

// Route is the routing decision produced by the router.
type Route string

const (
	// DONE means the residual contains no meaningful content and no further
	// analysis is required.
	DONE Route = "DONE"
	// CHECK_SMALL means the residual contains content that must be checked by
	// the small gate (and possibly the big model).
	CHECK_SMALL Route = "CHECK_SMALL"
)

// allowedDelimiters is the explicit set of characters that may remain in a
// residual without requiring further analysis. Letters, digits, unknown
// symbols, zero-width characters and anything outside this set require
// CHECK_SMALL.
var allowedDelimiters = map[rune]bool{
	'.': true, ',': true, ';': true, ':': true, '!': true, '?': true,
	'…': true, '-': true, '–': true, '—': true,
	'(': true, ')': true, '[': true, ']': true, '{': true, '}': true,
	'"': true, '\'': true, '«': true, '»': true,
}

// Router decides whether a residual text needs further analysis.
type Router struct{}

// NewRouter builds a Router.
func NewRouter() *Router {
	return &Router{}
}

// RouteForResidual returns the routing decision for a residual text.
func (r *Router) RouteForResidual(residual string) Route {
	for _, ch := range residual {
		if unicode.IsSpace(ch) {
			continue
		}
		if allowedDelimiters[ch] {
			continue
		}
		return CHECK_SMALL
	}
	return DONE
}