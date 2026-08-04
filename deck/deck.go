package deck

import (
	"errors"
	"math/rand"
)

type Card struct {
	rank string
	suit string
}

type Deck struct {
	cards []Card
}

func NewDeck() Deck {
	deck := Deck{}

	suits := []string{"Spades", "Hearts", "Diamonds", "Clubs"}
	ranks := []string{"Ace", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Jack", "Queen", "King"}

	for _, suit := range suits {
		for _, rank := range ranks {
			new_card := Card{
				suit: suit,
				rank: rank,
			}
			deck.cards = append(deck.cards, new_card)
		}
	}
	return deck
}

func (d *Deck) Shuffle() {
	rand.Shuffle(len(d.cards), func(i, j int) {
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	})
}

func (d *Deck) Deal() (Card, error) {
	if len(d.cards) == 0 {
		return Card{}, errors.New("Cannot deal. No more cards")
	}
	top_card := d.cards[0]
	d.cards = d.cards[1:]

	return top_card, nil
}
