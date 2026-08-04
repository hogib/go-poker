package deck

import (
	"fmt"
)

type Hand struct {
	Cards []Card
}

type Board struct {
	Cards []Card
}

func (h *Hand) AddCard(c Card) {
	h.Cards = append(h.Cards, c)
}

func (b *Board) AddCard(c Card) {
	b.Cards = append(b.Cards, c)
}

func DealHand(d *Deck) (Hand, error) {
	newHand := Hand{}

	for range 2 {
		card, err := d.Deal()
		if err != nil {
			return newHand, fmt.Errorf("not enough cards to deal hand: %w", err)
		}
		newHand.AddCard(card)
	}
	return newHand, nil
}

func DealFlop(d *Deck, b *Board) error {
	_, err := d.Deal()
	if err != nil {
		return fmt.Errorf("not enough cards to burn for flop: %w", err)
	}

	for range 3 {
		card, err := d.Deal()
		if err != nil {
			return fmt.Errorf("not enough cards for flop: %w", err)
		}
		b.AddCard(card)
	}
	return nil
}

func DealTurnOrRiver(d *Deck, b *Board) error {
	_, err := d.Deal()
	if err != nil {
		return fmt.Errorf("not enough cards to burn: %w", err)
	}

	card, err := d.Deal()
	if err != nil {
		return fmt.Errorf("not enough cards to deal: %w", err)
	}
	b.AddCard(card)

	return nil
}
