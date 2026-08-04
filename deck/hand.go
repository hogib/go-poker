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

type handActions interface {
	dealHand()
}

func (h *Hand) AddCard(c Card) {
	h.Cards = append(h.Cards, c)
}

func (b *Board) AddCard(c Card) {
	b.Cards = append(b.Cards, c)
}

func dealHand(d *Deck) (Hand, error) {
	newHand := Hand{}

	for i := 0; i < 5; i++ {
		card, err := d.Deal()
		if err != nil {
			return newHand, fmt.Errorf("not enough cards to deal %w", err)
		}
		newHand.Cards = append(newHand.Cards, card)

	}
	return newHand, nil
}

func dealFlop(d *Deck, b *Board) error {
	burned_deck := d.cards[1:]

	for i := 0; i < 3; i++ {
		card, err := d.Deal()
		if err != nil {
			return fmt.Errorf("Not enough cards to deal %w", err)
		}

		b.Cards = append(burned_deck, card)
	}

	return nil
}

func dealTurn(d *Deck, b *Board) error {
	burned_deck := d.cards[1:]

	for i := 0; i < 1; i++ {
		card, err := d.Deal()
		if err != nil {
			return fmt.Errorf("Not enough cards to deal %w", err)
		}

		b.Cards = append(burned_deck, card)
	}

	return nil
}

func dealRiver(d *Deck, b *Board) error {
	burned_deck := d.cards[1:]

	for i := 0; i < 1; i++ {
		card, err := d.Deal()
		if err != nil {
			return fmt.Errorf("Not enough cards to deal %w", err)
		}

		b.Cards = append(burned_deck, card)
	}

	return nil
}
