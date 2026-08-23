package game

import (
	"fmt"
	"go_poker/deck"
	"go_poker/player"
)

type Game struct {
	Players []*player.Player
	Deck    deck.Deck
	Board   deck.Board
	Pot     int

	SmallBlind int
	BigBlind   int

	ButtonIndex int
	CurrentBet  int
}

func NewGame(sb, bb int) Game {
	return Game{
		Players:     make([]*player.Player, 0, 9),
		SmallBlind:  sb,
		BigBlind:    bb,
		ButtonIndex: 0,
	}
}

func (g *Game) AddPlayer(p *player.Player) error {
	if len(g.Players) >= 9 {
		return fmt.Errorf("Table is full (max 9 players)")
	}

	g.Players = append(g.Players, p)
	return nil
}

func (g *Game) getBlindIndices() (int, int) {
	numPlayers := len(g.Players)

	if numPlayers == 2 {
		return g.ButtonIndex, (g.ButtonIndex + 1) % numPlayers
	}

	sbIndex := (g.ButtonIndex + 1) % numPlayers
	bbIndex := (g.ButtonIndex + 2) % numPlayers

	return sbIndex, bbIndex
}

func (g *Game) StartNewHand() error {
	if len(g.Players) < 2 {
		return fmt.Errorf("not enough players for new game")
	}

	g.Deck = deck.NewDeck()
	g.Board = deck.Board{}
	g.Pot = 0

	for _, p := range g.Players {
		err := p.ResetForNewHand(&g.Deck)

		if err != nil {
			return fmt.Errorf("failed to deal hand to %s: %w", p.Name, err)
		}
	}
	sbIndex, bbIndex := g.getBlindIndices()

	sbAmount, _ := g.Players[sbIndex].Bet(g.SmallBlind)
	bbAmount, _ := g.Players[bbIndex].Bet(g.BigBlind)

	g.Pot += sbAmount + bbAmount
	g.CurrentBet = g.BigBlind

	return nil
}

func (g *Game) MoveButton() {
	g.ButtonIndex = (g.ButtonIndex + 1) % len(g.Players)
}
