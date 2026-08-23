package player

import (
	"fmt"
	"go_poker/deck"
)

type Player struct {
	Name       string
	Hand       deck.Hand
	Chips      int
	CurrentBet int
	Folded     bool
}

func NewPlayer(name string, startingChips int) Player {
	return Player{
		Name:       name,
		Chips:      startingChips,
		CurrentBet: 0,
		Folded:     false,
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
	return amount, nil
}

func (p *Player) Fold() {
	p.Folded = true
}

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
	p.Folded = false
	return nil
}
