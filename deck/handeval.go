package deck

import (
	"sort"
)

const (
	StraightFlushMultiplier = 800
	QuadsMultiplier         = 700
	FullHouseMultiplier     = 600
	FlushMultiplier         = 500
	StraightMultiplier      = 400
	TripsMultiplier         = 300
	TwoPairMultiplier       = 200
	PairMultiplier          = 100
)

func getRankCounts(h *Hand) map[int]int {
	counts := make(map[int]int)
	for _, card := range h.Cards {
		counts[int(card.rank)]++
	}
	return counts
}

func buildScore(multiplier int, ranks []int) int {
	// Shift the hand multiplier into the highest bits
	score := multiplier << 20

	shift := 16
	for i := 0; i < 5; i++ {
		// Slide each card rank into the next available 4-bit slot
		score = score | (ranks[i] << shift)
		shift -= 4
	}

	return score
}

// rankSliceDesc returns this hand's ranks sorted high to low. It copies
// first: evaluation must never reorder the caller's cards.
func rankSliceDesc(h *Hand) []int {
	ranks := make([]int, 0, len(h.Cards))
	for _, card := range h.Cards {
		ranks = append(ranks, int(card.rank))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ranks)))
	return ranks
}

func isFlush(h *Hand) ([]int, bool) {
	if len(h.Cards) < 5 {
		return nil, false
	}
	firstSuit := h.Cards[0].suit

	for _, card := range h.Cards {
		if card.suit != firstSuit {
			return nil, false
		}
	}

	return rankSliceDesc(h), true
}

func isStraight(h *Hand) ([]int, bool) {
	if len(h.Cards) < 5 {
		return nil, false
	}

	ranks := rankSliceDesc(h)

	// wheel (ace low straight): A-5-4-3-2, where the ace plays low
	if ranks[0] == int(Ace) && ranks[1] == int(Five) && ranks[2] == int(Four) &&
		ranks[3] == int(Three) && ranks[4] == int(Two) {

		return []int{5, 4, 3, 2, 1}, true
	}

	for i := 1; i < len(ranks); i++ {
		if ranks[i] != ranks[i-1]-1 {
			return nil, false
		}
	}

	return ranks, true
}

func isTrips(h *Hand) ([]int, bool) {
	counts := getRankCounts(h)
	tripsRank := 0
	var kickers []int

	for rank, count := range counts {
		if count == 3 {
			tripsRank = rank
		} else if count == 1 {
			kickers = append(kickers, rank)
		}
	}

	if tripsRank > 0 && len(kickers) == 2 {
		if kickers[1] > kickers[0] {
			kickers[0], kickers[1] = kickers[1], kickers[0]
		}
		return []int{tripsRank, tripsRank, tripsRank, kickers[0], kickers[1]}, true
	}

	return nil, false
}

func isPair(h *Hand) ([]int, bool) {
	counts := getRankCounts(h)
	pairRank := 0
	var kickers []int

	for rank, count := range counts {
		if count == 2 {
			pairRank = rank
		} else if count == 1 {
			kickers = append(kickers, rank)
		}
	}

	if pairRank > 0 && len(kickers) >= 3 {
		sort.Slice(kickers, func(i, j int) bool {
			return kickers[i] > kickers[j]
		})

		ranks := []int{
			pairRank,
			pairRank,
			kickers[0],
			kickers[1],
			kickers[2],
		}

		return ranks, true
	}

	return nil, false
}

func isFourOfAKind(h *Hand) ([]int, bool) {
	counts := getRankCounts(h)
	quadRank, kicker := 0, 0

	for rank, count := range counts {
		if count == 4 {
			quadRank = rank
		} else if count == 1 {
			kicker = rank
		}
	}

	if quadRank > 0 {
		return []int{quadRank, quadRank, quadRank, quadRank, kicker}, true
	}

	return nil, false
}

func isFullHouse(h *Hand) ([]int, bool) {
	counts := getRankCounts(h)
	tripsRank, pairRank := 0, 0

	for rank, count := range counts {
		if count == 3 {
			tripsRank = rank
		} else if count == 2 {
			pairRank = rank
		}
	}

	if tripsRank > 0 && pairRank > 0 {
		return []int{tripsRank, tripsRank, tripsRank, pairRank, pairRank}, true
	}
	return nil, false
}

func isTwoPair(h *Hand) ([]int, bool) {
	counts := getRankCounts(h)
	var pairs []int
	kicker := 0

	for rank, count := range counts {
		if count == 2 {
			pairs = append(pairs, rank)
		} else if count == 1 {
			kicker = rank
		}
	}

	if len(pairs) == 2 {
		// Return the highest pair for tie-breaking
		if pairs[1] > pairs[0] {
			pairs[0], pairs[1] = pairs[1], pairs[0]
		}
		return []int{pairs[0], pairs[0], pairs[1], pairs[1], kicker}, true
	}
	return nil, false
}

func isStraightFlush(h *Hand) ([]int, bool) {
	if _, okF := isFlush(h); okF {
		if ranks, okS := isStraight(h); okS {
			return ranks, true
		}
	}
	return nil, false
}

func highCard(h *Hand) []int {
	return rankSliceDesc(h)
}

func EvaluateHand(h *Hand) int {
	if ranks, ok := isStraightFlush(h); ok {
		return buildScore(StraightFlushMultiplier, ranks)
	}
	if ranks, ok := isFourOfAKind(h); ok {
		return buildScore(QuadsMultiplier, ranks)
	}
	if ranks, ok := isFullHouse(h); ok {
		return buildScore(FullHouseMultiplier, ranks)
	}
	if ranks, ok := isFlush(h); ok {
		return buildScore(FlushMultiplier, ranks)
	}
	if ranks, ok := isStraight(h); ok {
		return buildScore(StraightMultiplier, ranks)
	}
	if ranks, ok := isTrips(h); ok {
		return buildScore(TripsMultiplier, ranks)
	}
	if ranks, ok := isTwoPair(h); ok {
		return buildScore(TwoPairMultiplier, ranks)
	}
	if ranks, ok := isPair(h); ok {
		return buildScore(PairMultiplier, ranks)
	}

	return buildScore(0, highCard(h))
}

// GetBestHand returns the highest-scoring five-card hand that can be made
// from the player's hole cards plus the board. Fewer than five cards
// available means there is nothing to score, so an empty hand comes back.
func GetBestHand(h *Hand, b *Board) Hand {
	var allCards []Card
	allCards = append(allCards, h.Cards...)
	allCards = append(allCards, b.Cards...)

	if len(allCards) < 5 {
		return Hand{}
	}

	var bestHand Hand
	highestScore := -1

	combo := make([]Card, 5)
	var walk func(start, filled int)
	walk = func(start, filled int) {
		if filled == 5 {
			candidate := Hand{Cards: append([]Card(nil), combo...)}
			if score := EvaluateHand(&candidate); score > highestScore {
				highestScore = score
				bestHand = candidate
			}
			return
		}
		// Stop early once too few cards remain to fill the hand.
		for i := start; i <= len(allCards)-(5-filled); i++ {
			combo[filled] = allCards[i]
			walk(i+1, filled+1)
		}
	}
	walk(0, 0)

	return bestHand
}
