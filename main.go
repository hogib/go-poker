package main

import (
	"fmt"
	"go_poker/deck"
)

func main() {
	playing_deck := deck.NewDeck()
	playing_deck.Shuffle()
	card, err := playing_deck.Deal()

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("your card iiissss", card)
}
