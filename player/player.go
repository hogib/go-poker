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

func NewPlayer(d *deck.Deck, name string, startingChips int) (Player, error) {
	startingHand, err := deck.DealHand(d)
	if err != nil {
		return Player{}, err
	}

	return Player{
		Name:       name,
		Hand:       startingHand,
		Chips:      startingChips,
		CurrentBet: 0,
		Folded:     false,
	}, nil
}

func (p *Player) Bet(amount int) error {
	if amount <= 0 {
		return fmt.Errorf("bet amount must be greater than zero")
	}

	if amount > p.Chips {
		amount = p.Chips
	}

	p.Chips -= amount
	p.CurrentBet += amount
	return nil
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
