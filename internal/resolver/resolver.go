// Package resolver resolves overlapping and duplicate candidate spans into a
// final set of entities. It prefers more specific/validated candidates over
// generic ones and is designed to be extensible.
package resolver

import (
	"sort"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// Resolver resolves candidate spans into final entities.
type Resolver struct {
	// minScore is the minimum score for an entity to be emitted.
	minScore float64
}

// New builds a resolver with the given minimum score threshold.
func New(minScore float64) *Resolver {
	return &Resolver{minScore: minScore}
}

// Resolve takes candidate spans and returns the final entities.
func (r *Resolver) Resolve(spans []entity.CandidateSpan) []entity.Entity {
	// 1. Drop candidates below the threshold.
	filtered := make([]entity.CandidateSpan, 0, len(spans))
	for _, s := range spans {
		if s.Score >= r.minScore {
			filtered = append(filtered, s)
		}
	}

	// 2. Sort by start, then by evidence strength descending, then score.
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Start != filtered[j].Start {
			return filtered[i].Start < filtered[j].Start
		}
		si, sj := filtered[i].EvidenceStrength(), filtered[j].EvidenceStrength()
		if si != sj {
			return si > sj
		}
		return filtered[i].Score > filtered[j].Score
	})

	// 3. Resolve overlaps greedily.
	var resolved []entity.CandidateSpan
	for _, s := range filtered {
		overlap := false
		for i, keep := range resolved {
			if spansOverlap(s, keep) {
				// Prefer the candidate with stronger evidence; tie-break by
				// score. This keeps a checksum+context candidate over a bare
				// regex candidate even if the regex candidate scored higher.
				if s.EvidenceStrength() > keep.EvidenceStrength() ||
					(s.EvidenceStrength() == keep.EvidenceStrength() && s.Score > keep.Score) {
					resolved[i] = s
				}
				overlap = true
				break
			}
		}
		if !overlap {
			resolved = append(resolved, s)
		}
	}

	// 4. Convert to entities.
	entities := make([]entity.Entity, 0, len(resolved))
	for _, s := range resolved {
		entities = append(entities, entity.Entity{
			Type:   s.Type,
			Text:   s.Text,
			Start:  s.Start,
			End:    s.End,
			Score:  s.Score,
			Reason: s.Reason,
		})
	}

	// 5. Sort by start for stable output.
	sort.SliceStable(entities, func(i, j int) bool {
		return entities[i].Start < entities[j].Start
	})
	return entities
}

// spansOverlap reports whether two spans overlap.
func spansOverlap(a, b entity.CandidateSpan) bool {
	return a.Start < b.End && b.Start < a.End
}
