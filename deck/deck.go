package deck

import (
	"errors"
	"math/rand"
)

type Suit int
type Rank int

const (
	Spades Suit = iota
	Hearts
	Diamonds
	Clubs
)

const (
	_     Rank = iota // 0
	_                 // 1
	Two               // 2
	Three             // 3
	Four              // 4
	Five              // 5
	Six               // 6
	Seven             // 7
	Eight             // 8
	Nine              // 9
	Ten               // 10
	Jack              // 11
	Queen             // 12
	King              // 13
	Ace               // 14
)

type Card struct {
	rank Rank
	suit Suit
}

type Deck struct {
	cards []Card
}

func NewDeck() Deck {
	deck := Deck{
		cards: make([]Card, 0, 52),
	}

	for s := Spades; s <= Clubs; s++ {
		for r := Two; r <= Ace; r++ {
			deck.cards = append(deck.cards, Card{
				suit: s,
				rank: r,
			})
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
