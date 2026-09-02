package player

import (
	"fmt"
	"ssh_holdem/deck"
)

type Player struct {
	Name       string
	Hand       deck.Hand
	Chips      int
	CurrentBet int // contributed on the current street
	TotalBet   int // contributed across the whole hand; drives side pots
	Folded     bool
	AllIn      bool
}

func NewPlayer(name string, startingChips int) Player {
	return Player{
		Name:       name,
		Chips:      startingChips,
		CurrentBet: 0,
		TotalBet:   0,
		Folded:     false,
		AllIn:      false,
	}
}

func (p *Player) Bet(amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("bet amount must be greater than zero")
	}

	if amount > p.Chips {
		amount = p.Chips
	}

	p.Chips -= amount
	p.CurrentBet += amount
	p.TotalBet += amount

	// Bet is the single clamp point, so it is also the only place that
	// needs to notice a player has committed their last chip.
	if p.Chips == 0 {
		p.AllIn = true
	}

	return amount, nil
}

// CanAct reports whether this player still has a decision to make.
// A folded or all-in player is skipped by the betting round.
func (p *Player) CanAct() bool {
	return !p.Folded && !p.AllIn && p.Chips > 0
}

// IsContesting reports whether this player can still win the pot,
// which includes players who are all-in and done acting.
func (p *Player) IsContesting() bool {
	return !p.Folded
}

func (p *Player) Fold() {
	p.Folded = true
}

// ResetForBettingRound clears the street-local bet only. TotalBet must
// survive across streets or side pots collapse into a single pot.
func (p *Player) ResetForBettingRound() {
	p.CurrentBet = 0
}

func (p *Player) ResetForNewHand(d *deck.Deck) error {
	newHand, err := deck.DealHand(d)

	if err != nil {
		return err
	}

	p.Hand = newHand
	p.CurrentBet = 0
	p.TotalBet = 0
	p.Folded = false
	p.AllIn = false
	return nil
}
