package game

import (
	"go_poker/deck"
)

// PotResult records how one pot layer was settled.
type PotResult struct {
	Amount  int
	Winners []int // seat indices sharing the pot
	Share   int   // chips each winner received before odd-chip handling

	// Best is the winning five-card hand. It is empty when the pot went
	// uncontested, since no hands were evaluated -- check
	// HandResult.Uncontested before rendering it.
	Best deck.Hand
}

// HandResult is everything a UI needs to narrate the end of a hand.
type HandResult struct {
	Board deck.Board
	Pots  []PotResult

	// PotTotal is what was in the middle before distribution. The
	// amounts in Pots must sum to it; a mismatch means the layering
	// dropped or invented chips.
	PotTotal int

	// Uncontested is true when everyone but one player folded, so no
	// hands were shown.
	Uncontested bool

	// Shown lists the seats whose cards went face up.
	Shown []int
}

// Payout derives the side pots and moves the chips. It is the only place
// chips leave the middle, so the conservation invariant lives or dies here.
func (g *Game) Payout() HandResult {
	pots := g.buildPots()
	uncontested := g.contestingCount() <= 1

	result := HandResult{
		Board:       g.Board,
		PotTotal:    g.Pot,
		Uncontested: uncontested,
		Pots:        make([]PotResult, 0, len(pots)),
	}

	// Evaluating once per seat rather than once per pot layer keeps the
	// combinatorial work off the inner loop.
	scores := make(map[int]int, len(g.Players))
	best := make(map[int]deck.Hand, len(g.Players))

	if !uncontested {
		for i, p := range g.Players {
			if !p.IsContesting() {
				continue
			}
			hand := deck.GetBestHand(&p.Hand, &g.Board)
			if len(hand.Cards) == 0 {
				continue
			}
			best[i] = hand
			scores[i] = deck.EvaluateHand(&hand)
			result.Shown = append(result.Shown, i)
		}
	}

	for _, pot := range pots {
		if len(pot.Eligible) == 0 {
			// Nobody left to claim it. This should not happen in a
			// well-formed hand, but silently dropping chips would be
			// worse than leaving them for the next pot layer.
			continue
		}

		winners := pot.Eligible
		if len(pot.Eligible) > 1 && !uncontested {
			winners = bestOf(pot.Eligible, scores)
		}

		share := pot.Amount / len(winners)
		remainder := pot.Amount % len(winners)

		for _, seat := range winners {
			g.Players[seat].Chips += share
		}

		// Odd chips go to the first winner left of the button, which is
		// the usual house rule and keeps payouts deterministic.
		for _, seat := range g.oddChipOrder(winners) {
			if remainder == 0 {
				break
			}
			g.Players[seat].Chips++
			remainder--
		}

		result.Pots = append(result.Pots, PotResult{
			Amount:  pot.Amount,
			Winners: winners,
			Share:   share,
			Best:    best[winners[0]],
		})
	}

	g.Pot = 0

	return result
}

func bestOf(seats []int, scores map[int]int) []int {
	topScore := -1
	var winners []int

	for _, seat := range seats {
		score, ok := scores[seat]
		if !ok {
			continue
		}
		if score > topScore {
			topScore = score
			winners = []int{seat}
		} else if score == topScore {
			winners = append(winners, seat)
		}
	}

	if len(winners) == 0 {
		return seats
	}
	return winners
}

// oddChipOrder walks the winners clockwise from the seat left of the button.
func (g *Game) oddChipOrder(winners []int) []int {
	n := len(g.Players)
	member := make(map[int]bool, len(winners))
	for _, seat := range winners {
		member[seat] = true
	}

	ordered := make([]int, 0, len(winners))
	for step := 1; step <= n; step++ {
		seat := (g.ButtonIndex + step) % n
		if member[seat] {
			ordered = append(ordered, seat)
		}
	}
	return ordered
}
