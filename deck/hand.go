package deck

import (
	"fmt"
)

type Hand struct {
	Cards []Card
}

func NewHand(d *Deck) (Hand, error) {
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
