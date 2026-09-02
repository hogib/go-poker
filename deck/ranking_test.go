package deck

import "testing"

func hand(cards ...Card) Hand { return Hand{Cards: cards} }

// c is shorthand for building a card in the fixtures below.
func c(r Rank, s Suit) Card { return Card{rank: r, suit: s} }

// The nine hand categories, weakest first. Anything in this list must
// beat everything above it, whatever the ranks involved -- so the strong
// hands here are deliberately made of low cards and the weak ones of
// aces and kings.
var categories = []struct {
	name string
	hand Hand
}{
	{"high card", hand(
		c(Ace, Spades), c(King, Hearts), c(Queen, Clubs), c(Jack, Diamonds), c(Nine, Spades),
	)},
	{"one pair", hand(
		c(Two, Spades), c(Two, Hearts), c(Five, Clubs), c(Seven, Diamonds), c(Nine, Spades),
	)},
	{"two pair", hand(
		c(Two, Spades), c(Two, Hearts), c(Three, Clubs), c(Three, Diamonds), c(Five, Spades),
	)},
	{"three of a kind", hand(
		c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Three, Diamonds), c(Five, Spades),
	)},
	{"straight", hand(
		c(Ace, Spades), c(Two, Hearts), c(Three, Clubs), c(Four, Diamonds), c(Five, Spades),
	)},
	{"flush", hand(
		c(Two, Spades), c(Three, Spades), c(Five, Spades), c(Seven, Spades), c(Nine, Spades),
	)},
	{"full house", hand(
		c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Three, Diamonds), c(Three, Spades),
	)},
	{"four of a kind", hand(
		c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Two, Diamonds), c(Three, Spades),
	)},
	{"straight flush", hand(
		c(Ace, Hearts), c(Two, Hearts), c(Three, Hearts), c(Four, Hearts), c(Five, Hearts),
	)},
}

// Every category must beat every weaker one. A low straight flush beats
// ace-high, and that has to hold for all thirty-six pairings, not just
// the ones a tie-break test happens to reach.
func TestHandCategoriesRankInOrder(t *testing.T) {
	for i := 0; i < len(categories); i++ {
		for j := i + 1; j < len(categories); j++ {
			weak, strong := categories[i], categories[j]

			weakScore := EvaluateHand(&weak.hand)
			strongScore := EvaluateHand(&strong.hand)

			if strongScore <= weakScore {
				t.Errorf("%s (%s, %d) should beat %s (%s, %d)",
					strong.name, strong.hand, strongScore,
					weak.name, weak.hand, weakScore)
			}
		}
	}
}

// Each category's detector must fire on its own hand, and the strongest
// matching one must be what EvaluateHand picks.
func TestEachCategoryIsDetected(t *testing.T) {
	detectors := map[string]func(*Hand) ([]int, bool){
		"one pair":        isPair,
		"two pair":        isTwoPair,
		"three of a kind": isTrips,
		"straight":        isStraight,
		"flush":           isFlush,
		"full house":      isFullHouse,
		"four of a kind":  isFourOfAKind,
		"straight flush":  isStraightFlush,
	}

	for _, tc := range categories {
		detect, ok := detectors[tc.name]
		if !ok {
			continue
		}
		h := tc.hand
		if _, matched := detect(&h); !matched {
			t.Errorf("%s was not detected in %s", tc.name, tc.hand)
		}
	}
}

// Detectors must not fire on hands that are not theirs. isPair in
// particular reads three kickers, and only the ordering inside
// EvaluateHand keeps two pair and full houses away from it.
func TestDetectorsRejectHandsThatAreNotTheirs(t *testing.T) {
	highCardHand := hand(
		c(Ace, Spades), c(King, Hearts), c(Queen, Clubs), c(Jack, Diamonds), c(Nine, Spades),
	)

	for _, tc := range []struct {
		name   string
		detect func(*Hand) ([]int, bool)
	}{
		{"isPair", isPair},
		{"isTwoPair", isTwoPair},
		{"isTrips", isTrips},
		{"isFullHouse", isFullHouse},
		{"isFourOfAKind", isFourOfAKind},
		{"isFlush", isFlush},
		{"isStraight", isStraight},
		{"isStraightFlush", isStraightFlush},
	} {
		h := highCardHand
		if _, matched := tc.detect(&h); matched {
			t.Errorf("%s matched a high-card hand", tc.name)
		}
	}

	// Two pair must not be read as one pair: that path indexes a third
	// kicker that does not exist.
	twoPair := hand(
		c(Two, Spades), c(Two, Hearts), c(Three, Clubs), c(Three, Diamonds), c(Five, Spades),
	)
	if _, matched := isPair(&twoPair); matched {
		t.Error("isPair matched two pair")
	}

	// A full house is not trips, and not a pair.
	boat := hand(
		c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Three, Diamonds), c(Three, Spades),
	)
	if _, matched := isTrips(&boat); matched {
		t.Error("isTrips matched a full house")
	}
	if _, matched := isPair(&boat); matched {
		t.Error("isPair matched a full house")
	}
}

// Detectors need five cards. Fewer must not panic or claim a match.
func TestDetectorsHandleShortHands(t *testing.T) {
	short := hand(c(Ace, Spades), c(Ace, Hearts), c(Ace, Clubs))

	for name, detect := range map[string]func(*Hand) ([]int, bool){
		"isFlush":         isFlush,
		"isStraight":      isStraight,
		"isStraightFlush": isStraightFlush,
	} {
		h := short
		if _, matched := detect(&h); matched {
			t.Errorf("%s matched a three-card hand", name)
		}
	}
}

// Within a category the ranks still decide it.
func TestWithinCategoryTheRanksDecide(t *testing.T) {
	for _, tc := range []struct {
		name          string
		better, worse Hand
	}{
		{
			"higher quads",
			hand(c(King, Spades), c(King, Hearts), c(King, Clubs), c(King, Diamonds), c(Two, Spades)),
			hand(c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Two, Diamonds), c(Ace, Spades)),
		},
		{
			"higher full house on the trips",
			hand(c(King, Spades), c(King, Hearts), c(King, Clubs), c(Two, Diamonds), c(Two, Spades)),
			hand(c(Two, Spades), c(Two, Hearts), c(Two, Clubs), c(Ace, Diamonds), c(Ace, Spades)),
		},
		{
			"higher flush",
			hand(c(Ace, Spades), c(Three, Spades), c(Five, Spades), c(Seven, Spades), c(Nine, Spades)),
			hand(c(King, Hearts), c(Three, Hearts), c(Five, Hearts), c(Seven, Hearts), c(Nine, Hearts)),
		},
		{
			"higher straight",
			hand(c(Ten, Spades), c(Jack, Hearts), c(Queen, Clubs), c(King, Diamonds), c(Ace, Spades)),
			hand(c(Nine, Spades), c(Ten, Hearts), c(Jack, Clubs), c(Queen, Diamonds), c(King, Spades)),
		},
		{
			"higher two pair on the top pair",
			hand(c(Ace, Spades), c(Ace, Hearts), c(Two, Clubs), c(Two, Diamonds), c(Three, Spades)),
			hand(c(King, Spades), c(King, Hearts), c(Queen, Clubs), c(Queen, Diamonds), c(Jack, Spades)),
		},
	} {
		better, worse := tc.better, tc.worse
		if EvaluateHand(&better) <= EvaluateHand(&worse) {
			t.Errorf("%s: %s should beat %s", tc.name, tc.better, tc.worse)
		}
	}
}

// Two identical holdings must score identically, whatever order the
// cards arrive in. Split pots depend on it.
func TestIdenticalHandsScoreEqually(t *testing.T) {
	one := hand(
		c(Ace, Spades), c(King, Hearts), c(Queen, Clubs), c(Jack, Diamonds), c(Ten, Spades),
	)
	other := hand(
		c(Ten, Hearts), c(Jack, Spades), c(Queen, Diamonds), c(King, Clubs), c(Ace, Hearts),
	)

	if EvaluateHand(&one) != EvaluateHand(&other) {
		t.Errorf("the same straight in a different order scored differently: %d vs %d",
			EvaluateHand(&one), EvaluateHand(&other))
	}
}

func TestDeckDealsEveryCardExactlyOnce(t *testing.T) {
	d := NewDeck()

	if d.Remaining() != 52 {
		t.Fatalf("a new deck should hold 52 cards, got %d", d.Remaining())
	}

	seen := make(map[Card]int, 52)
	for i := 0; i < 52; i++ {
		card, err := d.Deal()
		if err != nil {
			t.Fatalf("dealing card %d: %v", i, err)
		}
		seen[card]++
	}

	if len(seen) != 52 {
		t.Errorf("expected 52 distinct cards, got %d", len(seen))
	}
	for card, count := range seen {
		if count != 1 {
			t.Errorf("%s was dealt %d times", card, count)
		}
	}

	if _, err := d.Deal(); err == nil {
		t.Error("dealing from an empty deck should fail")
	}
}

func TestDealingRunsOutCleanly(t *testing.T) {
	d := Deck{}

	if _, err := DealHand(&d); err == nil {
		t.Error("DealHand should fail with no cards")
	}

	var b Board
	if err := DealFlop(&d, &b); err == nil {
		t.Error("DealFlop should fail with no cards")
	}
	if err := DealTurnOrRiver(&d, &b); err == nil {
		t.Error("DealTurnOrRiver should fail with no cards")
	}
	if len(b.Cards) != 0 {
		t.Errorf("a failed deal should leave the board alone, got %s", b)
	}
}

// Dealing a street burns a card first, which is what keeps the deck
// counts honest.
func TestStreetsBurnACard(t *testing.T) {
	d := NewDeck()
	var b Board

	if err := DealFlop(&d, &b); err != nil {
		t.Fatalf("DealFlop: %v", err)
	}
	if len(b.Cards) != 3 {
		t.Fatalf("the flop is three cards, got %d", len(b.Cards))
	}
	if got := d.Remaining(); got != 52-4 {
		t.Errorf("the flop should take four cards including the burn, %d left", got)
	}

	if err := DealTurnOrRiver(&d, &b); err != nil {
		t.Fatalf("DealTurnOrRiver: %v", err)
	}
	if len(b.Cards) != 4 {
		t.Fatalf("the turn adds one card, got a board of %d", len(b.Cards))
	}
	if got := d.Remaining(); got != 52-6 {
		t.Errorf("the turn should take two cards including the burn, %d left", got)
	}
}

func TestShuffleChangesTheOrder(t *testing.T) {
	a, b := NewDeck(), NewDeck()

	same := true
	for i := range a.cards {
		if a.cards[i] != b.cards[i] {
			same = false
			break
		}
	}

	if same {
		t.Error("two freshly shuffled decks came out in the same order")
	}
}

func TestSuitAndRankStrings(t *testing.T) {
	for suit, want := range map[Suit]string{
		Spades: "♠", Hearts: "♥", Diamonds: "♦", Clubs: "♣", Suit(99): "?",
	} {
		if got := suit.String(); got != want {
			t.Errorf("suit %d rendered %q, want %q", suit, got, want)
		}
	}

	for rank, want := range map[Rank]string{
		Two: "2", Nine: "9", Ten: "T", Jack: "J", Queen: "Q", King: "K", Ace: "A", Rank(0): "?",
	} {
		if got := rank.String(); got != want {
			t.Errorf("rank %d rendered %q, want %q", rank, got, want)
		}
	}
}

func TestHandAndBoardRender(t *testing.T) {
	h := hand(c(Ace, Spades), c(Ten, Hearts))
	if got := h.String(); got != "A♠ T♥" {
		t.Errorf("hand rendered %q", got)
	}

	b := Board{Cards: []Card{c(Two, Clubs), c(Three, Diamonds)}}
	if got := b.String(); got != "2♣ 3♦" {
		t.Errorf("board rendered %q", got)
	}

	if got := (Hand{}).String(); got != "" {
		t.Errorf("an empty hand should render as nothing, got %q", got)
	}
}
