package main

import (
	"fmt"
	"go_poker/deck"
)

func main() {
	playing_deck := deck.NewDeck()
	playing_deck.Shuffle()
	player1, err := deck.NewPlayer(&playing_deck)

	if err != nil {
		fmt.Errorf("error: %w", err)
		return
	}
	fmt.Println(player1.Hand)
}
