package game

import (
	"go_poker/player"
	"testing"
)

func TestAddPlayer(t *testing.T) {
	g := NewGame(5, 10)

	for i := 1; i <= 9; i++ {
		p := player.NewPlayer("TestPlayer", 1000)
		err := g.AddPlayer(&p)
		if err != nil {
			t.Errorf("Unexpected error adding player %d: %v", i, err)
		}
	}

	// Try to add a 10th player
	p10 := player.NewPlayer("TooMany", 1000)
	err := g.AddPlayer(&p10)
	if err == nil {
		t.Errorf("Expected an error when adding a 10th player, but got nil")
	}
}

func TestGetBlindIndices(t *testing.T) {
	// Test standard 3+ player wrapping
	g9 := NewGame(5, 10)
	for i := 0; i < 9; i++ {
		p := player.NewPlayer("Test", 100)
		g9.AddPlayer(&p)
	}

	g9.ButtonIndex = 8
	sb, bb := g9.getBlindIndices()

	if sb != 0 {
		t.Errorf("Expected SB to wrap around to seat 0, got %d", sb)
	}
	if bb != 1 {
		t.Errorf("Expected BB to wrap around to seat 1, got %d", bb)
	}

	// Test special Heads-Up (2 player) rules
	g2 := NewGame(5, 10)
	pA := player.NewPlayer("Alice", 100)
	pB := player.NewPlayer("Bob", 100)
	g2.AddPlayer(&pA)
	g2.AddPlayer(&pB)

	g2.ButtonIndex = 0
	sb2, bb2 := g2.getBlindIndices()
	if sb2 != 0 || bb2 != 1 {
		t.Errorf("Heads-Up Button 0 failed: expected SB 0 and BB 1, got SB %d and BB %d", sb2, bb2)
	}

	g2.ButtonIndex = 1
	sb2, bb2 = g2.getBlindIndices()
	if sb2 != 1 || bb2 != 0 {
		t.Errorf("Heads-Up Button 1 failed: expected SB 1 and BB 0, got SB %d and BB %d", sb2, bb2)
	}
}

func TestStartNewHand(t *testing.T) {
	g := NewGame(10, 20)
	p1 := player.NewPlayer("Alice", 1000)
	p2 := player.NewPlayer("Bob", 1000)
	p3 := player.NewPlayer("Charlie", 1000)

	g.AddPlayer(&p1)
	g.AddPlayer(&p2)
	g.AddPlayer(&p3)

	err := g.StartNewHand()
	if err != nil {
		t.Fatalf("Failed to start new hand: %v", err)
	}

	// Verify Pot and CurrentBet
	if g.Pot != 30 {
		t.Errorf("Expected pot to be 30 (10+20), got %d", g.Pot)
	}
	if g.CurrentBet != 20 {
		t.Errorf("Expected CurrentBet to be the BB (20), got %d", g.CurrentBet)
	}

	if g.Players[0].Chips != 1000 {
		t.Errorf("Expected Alice to have 1000 chips, got %d", g.Players[0].Chips)
	}
	if g.Players[1].Chips != 990 {
		t.Errorf("Expected Bob to have 990 chips, got %d", g.Players[1].Chips)
	}
	if g.Players[2].Chips != 980 {
		t.Errorf("Expected Charlie to have 980 chips, got %d", g.Players[2].Chips)
	}

	// Verify cards were dealt
	if len(g.Players[0].Hand.Cards) != 2 {
		t.Errorf("Expected Alice to receive 2 cards")
	}
}

func TestMoveButton(t *testing.T) {
	g := NewGame(5, 10)
	for i := 0; i < 3; i++ {
		p := player.NewPlayer("Test", 100)
		g.AddPlayer(&p)
	}

	g.ButtonIndex = 0
	g.MoveButton() // Should move to 1
	g.MoveButton() // Should move to 2
	g.MoveButton() // Should wrap back to 0

	if g.ButtonIndex != 0 {
		t.Errorf("Expected button to wrap back to 0, got %d", g.ButtonIndex)
	}
}
