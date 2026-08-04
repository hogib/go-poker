package main

import (
	"go_poker/deck"
)

func main() {
	playing_deck := deck.NewDeck()
	playing_deck.Shuffle()
	player1 := deck.Player{}

	player1.hand.dealHand()

}
