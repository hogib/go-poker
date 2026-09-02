package main

import (
	"fmt"
	"math/rand"

	"go_poker/game"
	"go_poker/player"
)

// A headless driver for the engine. The real entry point becomes the SSH
// server in phase 2; until then this is how you watch a hand play out.
func main() {
	g := game.NewGame(10, 20)

	names := []string{"Alice", "Bob", "Charlie", "Dana"}
	r := rand.New(rand.NewSource(1))

	for _, name := range names {
		p := player.NewPlayer(name, 1000)
		g.AddPlayerWithSource(&p, game.RandomBot{Rand: r})
	}

	for hand := 1; hand <= 5; hand++ {
		g.RemoveBustedPlayers()
		if len(g.Players) < 2 {
			break
		}

		result, err := g.PlayHand()
		if err != nil {
			fmt.Println("hand failed:", err)
			return
		}

		fmt.Printf("\n--- hand %d ---\n", hand)
		fmt.Printf("board: %s\n", result.Board)

		for _, pot := range result.Pots {
			names := make([]string, 0, len(pot.Winners))
			for _, seat := range pot.Winners {
				names = append(names, g.Players[seat].Name)
			}
			if result.Uncontested {
				fmt.Printf("%d to %v (everyone folded)\n", pot.Amount, names)
			} else {
				fmt.Printf("%d to %v with %s\n", pot.Amount, names, pot.Best)
			}
		}

		for _, p := range g.Players {
			fmt.Printf("  %-8s %5d\n", p.Name, p.Chips)
		}
	}
}
