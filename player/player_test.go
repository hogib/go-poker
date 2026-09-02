package player

import (
	"ssh_holdem/deck"
	"testing"
)

func setupTestDeck() *deck.Deck {
	d := deck.NewDeck()
	return &d
}

func TestNewPlayer(t *testing.T) {
	p := NewPlayer("Alice", 1000)

	if p.Name != "Alice" {
		t.Errorf("Expected name 'Alice', got %s", p.Name)
	}
	if p.Chips != 1000 {
		t.Errorf("Expected 1000 chips, got %d", p.Chips)
	}
	if p.CurrentBet != 0 {
		t.Errorf("Expected starting bet to be 0, got %d", p.CurrentBet)
	}
	if p.Folded != false {
		t.Errorf("Expected new player to not be folded")
	}
}

func TestPlayerBetting(t *testing.T) {
	p := NewPlayer("Bob", 500)

	_, err := p.Bet(100)
	if err != nil {
		t.Errorf("Unexpected error on valid bet: %v", err)
	}
	if p.Chips != 400 {
		t.Errorf("Expected 400 chips remaining, got %d", p.Chips)
	}
	if p.CurrentBet != 100 {
		t.Errorf("Expected current bet to be 100, got %d", p.CurrentBet)
	}

	_, err = p.Bet(1000)
	if err != nil {
		t.Errorf("Unexpected error on all-in bet: %v", err)
	}
	if p.Chips != 0 {
		t.Errorf("Expected 0 chips remaining after all-in, got %d", p.Chips)
	}
	if p.CurrentBet != 500 {
		t.Errorf("Expected total current bet to cap at 500, got %d", p.CurrentBet)
	}

	_, err = p.Bet(0)
	if err == nil {
		t.Errorf("Expected an error when betting 0, but got nil")
	}
}

func TestPlayerFold(t *testing.T) {
	p := Player{Name: "Charlie", Folded: false}
	p.Fold()

	if p.Folded != true {
		t.Errorf("Expected player to be marked as folded")
	}
}

func TestPlayerResets(t *testing.T) {
	d := setupTestDeck()
	p := NewPlayer("Dave", 1000)

	p.Bet(200)
	p.Fold()

	p.ResetForBettingRound()
	if p.CurrentBet != 0 {
		t.Errorf("Expected CurrentBet to be cleared for the new round, got %d", p.CurrentBet)
	}
	if p.Folded != true {
		t.Errorf("CRITICAL BUG: Player was resurrected! They should still be folded.")
	}

	err := p.ResetForNewHand(d)
	if err != nil {
		t.Fatalf("Failed to reset for new hand: %v", err)
	}
	if p.Folded != false {
		t.Errorf("Expected Folded state to be reset to false for a new game")
	}
	if len(p.Hand.Cards) != 2 {
		t.Errorf("Expected player to receive 2 new cards")
	}
}
