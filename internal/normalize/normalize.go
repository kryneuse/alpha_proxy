// Package normalize provides a normalization layer that produces a
// normalized representation of the input text while preserving the ability
// to map any normalized position back to the ORIGINAL text offsets.
package normalize

import (
	"strings"
	"unicode"
)

// Text is a normalized view of the original input. It keeps the original
// string and a per-rune mapping from normalized positions to original
// byte offsets.
type Text struct {
	Original string
	// Normalized is the transformed string (lowercased, unicode dashes
	// replaced, whitespace collapsed).
	Normalized string
	// origOffsets[i] is the byte offset in Original of the rune at
	// normalized position i. It has length len([]rune(Normalized)).
	origOffsets []int
	// origEnds[i] is the byte offset just after the rune at normalized
	// position i.
	origEnds []int
}

// New builds a normalized Text from the original string.
//
// Normalization applied:
//   - lowercasing (unicode-aware)
//   - replacing unicode dashes/hyphens with ASCII '-'
//   - collapsing runs of whitespace into a single space
//
// The mapping preserves the position of the first rune of each normalized
// rune, which is sufficient for span mapping because normalization never
// reorders or duplicates runes.
func New(original string) *Text {
	origRunes := []rune(original)
	var norm strings.Builder
	offsets := make([]int, 0, len(origRunes))
	ends := make([]int, 0, len(origRunes))

	byteOff := 0
	prevSpace := false
	for _, r := range origRunes {
		rlen := len(string(r))
		switch {
		case isDash(r):
			norm.WriteRune('-')
			offsets = append(offsets, byteOff)
			ends = append(ends, byteOff+rlen)
			byteOff += rlen
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace {
				norm.WriteRune(' ')
				offsets = append(offsets, byteOff)
				ends = append(ends, byteOff+rlen)
			}
			byteOff += rlen
			prevSpace = true
		default:
			norm.WriteRune(unicode.ToLower(r))
			offsets = append(offsets, byteOff)
			ends = append(ends, byteOff+rlen)
			byteOff += rlen
			prevSpace = false
		}
	}

	return &Text{
		Original:    original,
		Normalized:  norm.String(),
		origOffsets: offsets,
		origEnds:    ends,
	}
}

// isDash reports whether r is a unicode dash or hyphen-like character.
func isDash(r rune) bool {
	switch r {
	case '-', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015',
		'\u2212', '\uFE58', '\uFE63', '\uFF0D', '\u00AD':
		return true
	}
	return false
}

// MapSpan maps a normalized [start,end) rune range back to original byte
// offsets. It returns the original byte start and end.
func (t *Text) MapSpan(normStart, normEnd int) (int, int) {
	if len(t.origOffsets) == 0 {
		return 0, 0
	}
	if normStart < 0 {
		normStart = 0
	}
	if normEnd > len(t.origOffsets) {
		normEnd = len(t.origOffsets)
	}
	if normStart >= len(t.origOffsets) {
		return len(t.Original), len(t.Original)
	}
	start := t.origOffsets[normStart]
	end := len(t.Original)
	if normEnd > 0 && normEnd <= len(t.origEnds) {
		end = t.origEnds[normEnd-1]
	}
	return start, end
}

// OriginalSlice returns the original substring for a normalized span.
func (t *Text) OriginalSlice(normStart, normEnd int) string {
	s, e := t.MapSpan(normStart, normEnd)
	return t.Original[s:e]
}

// Len returns the length in runes of the normalized text.
func (t *Text) Len() int {
	return len(t.origOffsets)
}

// ByteToRune converts a byte offset in the normalized string to a rune index.
func (t *Text) ByteToRune(byteOff int) int {
	return len([]rune(t.Normalized[:byteOff]))
}

// OriginalToNorm maps an original byte offset to the nearest normalized rune
// index. It returns the index of the first normalized rune whose original
// offset is >= origOffset.
func (t *Text) OriginalToNorm(origOffset int) int {
	lo, hi := 0, len(t.origOffsets)
	for lo < hi {
		mid := (lo + hi) / 2
		if t.origOffsets[mid] < origOffset {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
