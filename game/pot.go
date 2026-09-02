package game

import "sort"

// Pot is one layer of the pot: an amount, and the seats still contesting
// it. Layers exist because a player who is all-in for less than the bet
// cannot win the chips wagered above their stack.
type Pot struct {
	Amount   int
	Eligible []int // seat indices, ascending
}

// buildPots derives the pot layers from what each player has committed
// this hand. It is a pure function of every player's TotalBet and Folded
// flag, which is why the engine tracks a single running Pot total and only
// works out the layering at distribution time.
//
// For each distinct contribution level, the layer holds the slice of chips
// between that level and the one below it, taken from every player who
// reached it -- folded players included, since their chips stay in the pot.
// Only players still contesting the hand are eligible to win it.
func (g *Game) buildPots() []Pot {
	levels := make([]int, 0, len(g.Players))
	seen := make(map[int]bool, len(g.Players))

	for _, p := range g.Players {
		if p.TotalBet > 0 && !seen[p.TotalBet] {
			seen[p.TotalBet] = true
			levels = append(levels, p.TotalBet)
		}
	}
	sort.Ints(levels)

	pots := make([]Pot, 0, len(levels))
	previous := 0

	// Dead money: chips at a level where every contributor has since
	// folded. Nobody can win them on their own terms, so they join the
	// layer below, or the first live layer above if there is none yet.
	carry := 0

	for _, level := range levels {
		contributors := 0
		var eligible []int

		for i, p := range g.Players {
			if p.TotalBet >= level {
				contributors++
				if p.IsContesting() {
					eligible = append(eligible, i)
				}
			}
		}

		amount := (level - previous) * contributors
		previous = level

		if amount == 0 {
			continue
		}

		if len(eligible) == 0 {
			if n := len(pots); n > 0 {
				pots[n-1].Amount += amount
			} else {
				carry += amount
			}
			continue
		}

		amount += carry
		carry = 0

		// Adjacent layers with the same contestants pay out identically,
		// so fold them together rather than reporting them separately.
		if n := len(pots); n > 0 && sameSeats(pots[n-1].Eligible, eligible) {
			pots[n-1].Amount += amount
			continue
		}

		pots = append(pots, Pot{Amount: amount, Eligible: eligible})
	}

	if carry > 0 && len(pots) > 0 {
		pots[len(pots)-1].Amount += carry
	}

	return pots
}

func sameSeats(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
