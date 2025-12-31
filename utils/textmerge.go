package utils

import (
	"strings"

	censor "github.com/heibot/censor"
)

// MergedText represents the result of merging multiple texts.
type MergedText struct {
	Merged string      // The merged text
	Parts  []string    // Original parts
	Index  []PartIndex // Index mapping for each part
}

// PartIndex represents the position of a part in the merged text.
type PartIndex struct {
	Start int // Start position in merged text
	End   int // End position in merged text
}

// MergeTexts merges multiple text parts into a single text.
// Returns the merged text and whether merging was successful.
func MergeTexts(parts []string, strategy censor.TextMergeStrategy) (MergedText, bool) {
	if len(parts) == 0 {
		return MergedText{}, false
	}

	if len(parts) == 1 {
		return MergedText{
			Merged: parts[0],
			Parts:  parts,
			Index: []PartIndex{
				{Start: 0, End: len(parts[0])},
			},
		}, true
	}

	// Calculate total length
	totalLen := 0
	for i, p := range parts {
		totalLen += len(p)
		if i > 0 {
			totalLen += len(strategy.Separator)
		}
	}

	// Check if we exceed max length
	if totalLen > strategy.MaxLen {
		// Try to merge as many as we can
		return mergePartial(parts, strategy)
	}

	// Merge all parts
	var builder strings.Builder
	builder.Grow(totalLen)

	indices := make([]PartIndex, len(parts))
	pos := 0

	for i, p := range parts {
		if i > 0 {
			builder.WriteString(strategy.Separator)
			pos += len(strategy.Separator)
		}

		indices[i] = PartIndex{
			Start: pos,
			End:   pos + len(p),
		}

		builder.WriteString(p)
		pos += len(p)
	}

	return MergedText{
		Merged: builder.String(),
		Parts:  parts,
		Index:  indices,
	}, true
}

// mergePartial merges as many parts as possible within the max length.
func mergePartial(parts []string, strategy censor.TextMergeStrategy) (MergedText, bool) {
	var builder strings.Builder
	var indices []PartIndex
	var includedParts []string

	pos := 0
	for i, p := range parts {
		addLen := len(p)
		if i > 0 {
			addLen += len(strategy.Separator)
		}

		if pos+addLen > strategy.MaxLen {
			// Cannot include this part
			break
		}

		if i > 0 {
			builder.WriteString(strategy.Separator)
			pos += len(strategy.Separator)
		}

		indices = append(indices, PartIndex{
			Start: pos,
			End:   pos + len(p),
		})

		builder.WriteString(p)
		pos += len(p)
		includedParts = append(includedParts, p)
	}

	// If we couldn't include at least 2 parts, don't merge
	if len(includedParts) < 2 {
		return MergedText{}, false
	}

	return MergedText{
		Merged: builder.String(),
		Parts:  includedParts,
		Index:  indices,
	}, true
}

// SplitMergedText splits a merged text back into its original parts.
func SplitMergedText(merged MergedText) []string {
	result := make([]string, len(merged.Index))
	for i, idx := range merged.Index {
		if idx.End <= len(merged.Merged) {
			result[i] = merged.Merged[idx.Start:idx.End]
		}
	}
	return result
}

// FindViolatingParts identifies which parts contain a violation.
// The violation position is relative to the merged text.
func FindViolatingParts(merged MergedText, violationStart, violationEnd int) []int {
	var violatingParts []int

	for i, idx := range merged.Index {
		// Check if this part overlaps with the violation
		if idx.Start < violationEnd && idx.End > violationStart {
			violatingParts = append(violatingParts, i)
		}
	}

	return violatingParts
}

// TruncateText truncates text to a maximum length with ellipsis.
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	if maxLen <= 3 {
		return text[:maxLen]
	}

	return text[:maxLen-3] + "..."
}

// MaskText masks sensitive parts of text.
func MaskText(text string, start, end int, maskChar rune) string {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return text
	}

	runes := []rune(text)
	for i := start; i < end && i < len(runes); i++ {
		runes[i] = maskChar
	}

	return string(runes)
}
