package game

import (
	"fmt"
	"go_poker/deck"
	"go_poker/player"
)

type Action string

const (
	Fold  Action = "FOLD"
	Call  Action = "CALL"
	Raise Action = "RAISE"
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
	MinRaise    int
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
	g.MinRaise = g.BigBlind

	return nil
}

func (g *Game) getMockPlayerAction(p *player.Player) (Action, int) {
	toCall := g.CurrentBet - p.CurrentBet

	if toCall == 0 {
		return Call, 0
	}

	return Call, 0
}

func (g *Game) MoveButton() {
	g.ButtonIndex = (g.ButtonIndex + 1) % len(g.Players)
}

func (g *Game) ExecuteBettingRound(StartIndex int) {
	playersActed := 0
	currentIndex := StartIndex

	for {
		activeCount := 0

		for _, p := range g.Players {
			if !p.Folded {
				activeCount++
			}
		}

		if activeCount == 1 {
			return
		}

		bettingMatched := true
		activePlayersInRound := 0

		for _, p := range g.Players {
			if !p.Folded && p.Chips > 0 {
				activePlayersInRound++
				if p.CurrentBet != g.CurrentBet {
					bettingMatched = false
				}
			}
		}

		if playersActed >= activePlayersInRound && bettingMatched {
			return
		}

		p := g.Players[currentIndex]

		if p.Folded || p.Chips == 0 {
			currentIndex = (currentIndex + 1) % len(g.Players)
			continue
		}

		action, raiseAmount := g.getMockPlayerAction(p)

		switch action {
		case Fold:
			p.Fold()

		case Call:
			toCall := g.CurrentBet - p.CurrentBet

			if toCall > 0 {
				actualBet, _ := p.Bet(toCall)
				g.Pot += actualBet
			}

		case Raise:
			toCall := g.CurrentBet - p.CurrentBet
			totalDeduction := toCall + raiseAmount
			if raiseAmount < g.MinRaise && totalDeduction < p.Chips {
				fmt.Printf("Illegal raise amount. must be at least %d.\n", g.MinRaise)
				continue
			}
			actualBet, _ := p.Bet(totalDeduction)
			g.Pot += actualBet

			if p.CurrentBet > g.CurrentBet {
				if raiseAmount >= g.MinRaise {
					g.MinRaise = raiseAmount
				}

				g.CurrentBet = p.CurrentBet
				playersActed = 0
			}

		}

		playersActed++
		currentIndex = (currentIndex + 1) % len(g.Players)

	}
}
