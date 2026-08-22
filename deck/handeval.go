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

	sort.Slice(h.Cards, func(i, j int) bool {
		return h.Cards[i].rank < h.Cards[j].rank
	})

	ranks := []int{
		int(h.Cards[4].rank),
		int(h.Cards[3].rank),
		int(h.Cards[2].rank),
		int(h.Cards[1].rank),
		int(h.Cards[0].rank),
	}

	return ranks, true
}

func isStraight(h *Hand) ([]int, bool) {
	if len(h.Cards) < 5 {
		return nil, false
	}

	sort.Slice(h.Cards, func(i, j int) bool {
		return h.Cards[i].rank < h.Cards[j].rank
	})
	// wheel (ace low straight)
	if h.Cards[0].rank == Two && h.Cards[1].rank == Three &&
		h.Cards[2].rank == Four && h.Cards[3].rank == Five &&
		h.Cards[4].rank == Ace {

		return []int{5, 4, 3, 2, 1}, true
	}

	for i := 1; i < len(h.Cards); i++ {
		if h.Cards[i].rank != h.Cards[i-1].rank+1 {
			return nil, false
		}

	}

	ranks := []int{
		int(h.Cards[4].rank),
		int(h.Cards[3].rank),
		int(h.Cards[2].rank),
		int(h.Cards[1].rank),
		int(h.Cards[0].rank),
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

	if pairRank > 0 {
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
	highest := 0

	sort.Slice(h.Cards, func(i, j int) bool {
		return h.Cards[i].rank < h.Cards[j].rank
	})

	highest = int(h.Cards[4].rank)

	return []int{highest, int(h.Cards[3].rank), int(h.Cards[2].rank), int(h.Cards[1].rank), int(h.Cards[0].rank)}
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

func GetBestHand(h *Hand, b *Board) Hand {
	var allCards []Card
	allCards = append(allCards, h.Cards...)
	allCards = append(allCards, b.Cards...)

	var bestHand Hand
	var highestScore int

	for i := 0; i < len(allCards)-1; i++ {
		for j := i + 1; j < len(allCards); j++ {

			combo := Hand{Cards: make([]Card, 0, 5)}
			for k, card := range allCards {
				if k != i && k != j {
					combo.AddCard(card)
				}
			}

			score := EvaluateHand(&combo)

			if score > highestScore {
				highestScore = score
				bestHand = combo
			}
		}
	}

	return bestHand
}
