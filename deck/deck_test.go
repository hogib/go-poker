package deck

import (
	"testing"
)

func TestEvaluateHandTieBreakers(t *testing.T) {
	tests := []struct {
		name         string
		hand1        Hand
		hand2        Hand
		wantHand1Win bool
	}{
		{
			name: "Pair with Ace Kicker vs Pair with King Kicker",
			hand1: Hand{Cards: []Card{
				{suit: Spades, rank: Eight}, {suit: Clubs, rank: Eight},
				{suit: Hearts, rank: Ace}, {suit: Diamonds, rank: Ten}, {suit: Spades, rank: Two},
			}}, // 8-8-A-10-2
			hand2: Hand{Cards: []Card{
				{suit: Diamonds, rank: Eight}, {suit: Hearts, rank: Eight},
				{suit: Hearts, rank: King}, {suit: Diamonds, rank: Queen}, {suit: Spades, rank: Jack},
			}}, // 8-8-K-Q-J
			wantHand1Win: true,
		},
		{
			name: "Two Pair - Kicker Tie Breaker",
			hand1: Hand{Cards: []Card{
				{suit: Spades, rank: Ace}, {suit: Clubs, rank: Ace},
				{suit: Hearts, rank: Eight}, {suit: Diamonds, rank: Eight}, {suit: Spades, rank: King},
			}}, // A-A-8-8-K
			hand2: Hand{Cards: []Card{
				{suit: Diamonds, rank: Ace}, {suit: Hearts, rank: Ace},
				{suit: Clubs, rank: Eight}, {suit: Spades, rank: Eight}, {suit: Spades, rank: Queen},
			}}, // A-A-8-8-Q
			wantHand1Win: true,
		},
		{
			name: "Flush Tie - Ace High vs King High",
			hand1: Hand{Cards: []Card{
				{suit: Hearts, rank: Ace}, {suit: Hearts, rank: King},
				{suit: Hearts, rank: Jack}, {suit: Hearts, rank: Five}, {suit: Hearts, rank: Two},
			}}, // Ah-Kh-Jh-5h-2h
			hand2: Hand{Cards: []Card{
				{suit: Diamonds, rank: King}, {suit: Diamonds, rank: Queen},
				{suit: Diamonds, rank: Jack}, {suit: Diamonds, rank: Ten}, {suit: Diamonds, rank: Eight},
			}}, // Kd-Qd-Jd-10d-8d
			wantHand1Win: true,
		},
		{
			name: "High Card - Deep 5th Card Tie Breaker",
			hand1: Hand{Cards: []Card{
				{suit: Spades, rank: Ace}, {suit: Diamonds, rank: King},
				{suit: Clubs, rank: Jack}, {suit: Hearts, rank: Nine}, {suit: Spades, rank: Four},
			}}, // A-K-J-9-4
			hand2: Hand{Cards: []Card{
				{suit: Hearts, rank: Ace}, {suit: Spades, rank: King},
				{suit: Hearts, rank: Jack}, {suit: Diamonds, rank: Nine}, {suit: Clubs, rank: Three},
			}}, // A-K-J-9-3
			wantHand1Win: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score1 := EvaluateHand(&tt.hand1)
			score2 := EvaluateHand(&tt.hand2)

			// Check if Hand 1 correctly scored higher than Hand 2
			hand1Won := score1 > score2

			if hand1Won != tt.wantHand1Win {
				t.Errorf("\nFailed: %s\nHand 1 Score: %d\nHand 2 Score: %d\nExpected Hand 1 to win: %v, got: %v",
					tt.name, score1, score2, tt.wantHand1Win, hand1Won)
			}
		})
	}
}
