package main

import (
	"go_poker/deck"
)

func main() {
	playing_deck := deck.NewDeck()
	playing_deck.Shuffle()

}
