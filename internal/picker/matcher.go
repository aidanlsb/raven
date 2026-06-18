package picker

import (
	"math"
	"sort"
	"strings"
)

const (
	matchCharScore       = 1
	matchBoundaryBonus   = 12
	matchAdjacentBonus   = 8
	matchExactTokenBonus = 20
	matchGapPenalty      = 2
	matchLeadingPenalty  = 1
	matchMaxPenalty      = 24
)

type rankedItem struct {
	index     int
	score     int
	targetLen int
}

type matchState struct {
	score int
	index int
	start int
}

func rankItems(items []Item, query string) []int {
	tokens := strings.Fields(normalizeFuzzyText(query))
	if len(tokens) == 0 {
		indexes := make([]int, len(items))
		for i := range items {
			indexes[i] = i
		}
		return indexes
	}

	matches := make([]rankedItem, 0, len(items))
	for index, item := range items {
		target := normalizeFuzzyText(item.searchText())
		score, ok := scoreTarget(target, tokens)
		if !ok {
			continue
		}
		matches = append(matches, rankedItem{
			index:     index,
			score:     score,
			targetLen: len([]rune(target)),
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].targetLen < matches[j].targetLen
	})

	indexes := make([]int, 0, len(matches))
	for _, match := range matches {
		indexes = append(indexes, match.index)
	}
	return indexes
}

func scoreTarget(target string, tokens []string) (int, bool) {
	score := 0
	for _, token := range tokens {
		tokenScore, ok := scoreToken(target, token)
		if !ok {
			return 0, false
		}
		score += tokenScore
	}
	return score, true
}

func scoreToken(target, token string) (int, bool) {
	targetRunes := []rune(target)
	tokenRunes := []rune(token)
	if len(tokenRunes) == 0 {
		return 0, true
	}
	if len(targetRunes) == 0 {
		return 0, false
	}

	prev := make([]matchState, 0, len(targetRunes))
	for targetIndex, targetRune := range targetRunes {
		if targetRune != tokenRunes[0] {
			continue
		}
		score := matchCharScore + boundaryBonus(targetRunes, targetIndex)
		score -= cappedPenalty(targetIndex * matchLeadingPenalty)
		prev = append(prev, matchState{score: score, index: targetIndex, start: targetIndex})
	}
	if len(prev) == 0 {
		return 0, false
	}

	for tokenIndex := 1; tokenIndex < len(tokenRunes); tokenIndex++ {
		curr := make([]matchState, 0, len(targetRunes))
		for targetIndex, targetRune := range targetRunes {
			if targetRune != tokenRunes[tokenIndex] {
				continue
			}
			best := matchState{score: math.MinInt}
			for _, candidate := range prev {
				if candidate.index >= targetIndex {
					continue
				}
				gap := targetIndex - candidate.index - 1
				score := candidate.score + matchCharScore + boundaryBonus(targetRunes, targetIndex)
				if gap == 0 {
					score += matchAdjacentBonus
				} else {
					score -= cappedPenalty(gap * matchGapPenalty)
				}
				if score > best.score {
					best = matchState{score: score, index: targetIndex, start: candidate.start}
				}
			}
			if best.score != math.MinInt {
				curr = append(curr, best)
			}
		}
		if len(curr) == 0 {
			return 0, false
		}
		prev = curr
	}

	best := matchState{score: math.MinInt}
	for _, candidate := range prev {
		score := candidate.score
		if candidate.index-candidate.start+1 == len(tokenRunes) {
			score += matchExactTokenBonus
		}
		if score > best.score {
			best = candidate
			best.score = score
		}
	}
	if best.score == math.MinInt {
		return 0, false
	}
	return best.score, true
}

func boundaryBonus(target []rune, index int) int {
	if index == 0 || target[index-1] == ' ' {
		return matchBoundaryBonus
	}
	return 0
}

func cappedPenalty(penalty int) int {
	if penalty > matchMaxPenalty {
		return matchMaxPenalty
	}
	return penalty
}
