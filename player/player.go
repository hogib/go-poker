package player

import (
	"go_poker/deck"
)

type Player struct {
	Hand deck.Hand
}

func newPlayer(d *deck.Deck) (Player, error) {
	startingHand, err := deck.DealHand(d)
	if err != nil {
		return Player{}, err
	}

	return Player{
		Hand: startingHand,
	}, nil
}
