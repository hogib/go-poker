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

	deck.Shuffle()

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

func (s Suit) String() string {
	switch s {
	case Spades:
		return "♠"
	case Hearts:
		return "♥"
	case Diamonds:
		return "♦"
	case Clubs:
		return "♣"
	}
	return "?"
}

func (r Rank) String() string {
	switch r {
	case Ten:
		return "T"
	case Jack:
		return "J"
	case Queen:
		return "Q"
	case King:
		return "K"
	case Ace:
		return "A"
	}
	if r < Two || r > Ace {
		return "?"
	}
	return string(rune('0' + int(r)))
}

func (c Card) String() string {
	return c.rank.String() + c.suit.String()
}

// Rank exposes the card's rank to callers outside this package.
func (c Card) Rank() Rank { return c.rank }

// Suit exposes the card's suit to callers outside this package.
func (c Card) Suit() Suit { return c.suit }

// NewCard builds a card, mainly so tests and fixtures outside this
// package can construct known holdings.
func NewCard(r Rank, s Suit) Card {
	return Card{rank: r, suit: s}
}

// Remaining reports how many cards are left undealt.
func (d *Deck) Remaining() int { return len(d.cards) }
