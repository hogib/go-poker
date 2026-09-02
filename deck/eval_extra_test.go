package deck

import "testing"

func TestEvaluateHandDoesNotMutate(t *testing.T) {
	h := Hand{Cards: []Card{
		{suit: Spades, rank: Two}, {suit: Hearts, rank: Ace},
		{suit: Clubs, rank: Seven}, {suit: Diamonds, rank: King},
		{suit: Spades, rank: Nine},
	}}
	before := append([]Card(nil), h.Cards...)

	EvaluateHand(&h)

	for i := range before {
		if h.Cards[i] != before[i] {
			t.Fatalf("EvaluateHand reordered the hand: index %d was %v, now %v",
				i, before[i], h.Cards[i])
		}
	}
}

func TestGetBestHandPicksFiveOfSeven(t *testing.T) {
	hole := Hand{Cards: []Card{
		{suit: Spades, rank: Ace}, {suit: Spades, rank: King},
	}}
	board := Board{Cards: []Card{
		{suit: Spades, rank: Queen}, {suit: Spades, rank: Jack},
		{suit: Spades, rank: Ten}, {suit: Hearts, rank: Two},
		{suit: Clubs, rank: Two},
	}}

	best := GetBestHand(&hole, &board)
	if len(best.Cards) != 5 {
		t.Fatalf("expected a 5-card hand, got %d", len(best.Cards))
	}

	got := EvaluateHand(&best)
	want := buildScore(StraightFlushMultiplier, []int{14, 13, 12, 11, 10})
	if got != want {
		t.Errorf("expected the royal flush (score %d), got %d for %s", want, got, best)
	}
}

func TestGetBestHandOnFlop(t *testing.T) {
	// Five cards available: the whole holding is the best hand,
	// and nothing should panic.
	hole := Hand{Cards: []Card{
		{suit: Spades, rank: Ace}, {suit: Hearts, rank: Ace},
	}}
	board := Board{Cards: []Card{
		{suit: Clubs, rank: Ace}, {suit: Diamonds, rank: Four},
		{suit: Hearts, rank: Nine},
	}}

	best := GetBestHand(&hole, &board)
	if len(best.Cards) != 5 {
		t.Fatalf("expected a 5-card hand, got %d", len(best.Cards))
	}
	if _, ok := isTrips(&best); !ok {
		t.Errorf("expected trip aces, got %s", best)
	}
}

func TestWheelStraightRanksBelowSixHigh(t *testing.T) {
	wheel := Hand{Cards: []Card{
		{suit: Spades, rank: Ace}, {suit: Hearts, rank: Two},
		{suit: Clubs, rank: Three}, {suit: Diamonds, rank: Four},
		{suit: Spades, rank: Five},
	}}
	sixHigh := Hand{Cards: []Card{
		{suit: Spades, rank: Two}, {suit: Hearts, rank: Three},
		{suit: Clubs, rank: Four}, {suit: Diamonds, rank: Five},
		{suit: Spades, rank: Six},
	}}

	if EvaluateHand(&wheel) >= EvaluateHand(&sixHigh) {
		t.Errorf("the wheel must lose to a six-high straight")
	}
}

func TestCardString(t *testing.T) {
	cases := map[Card]string{
		{rank: Ace, suit: Spades}:   "A♠",
		{rank: Ten, suit: Hearts}:   "T♥",
		{rank: Two, suit: Diamonds}: "2♦",
		{rank: Nine, suit: Clubs}:   "9♣",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	}
}
