package game

import (
	"context"
	"time"

	"ssh_holdem/deck"
)

type Action string

const (
	Fold  Action = "FOLD"
	Check Action = "CHECK"
	Call  Action = "CALL"
	Raise Action = "RAISE"
)

// Decision is one player's answer when it is their turn.
//
// Amount is meaningful only for Raise, and is a "raise to" figure: the
// total this player wants their bet for the current street to be, not the
// increment above the current bet. Engines that use the increment tend to
// grow off-by-one bugs at every call site, so the total is the contract.
type Decision struct {
	Action Action
	Amount int
}

// ActionSource supplies decisions for one seat. A human on an SSH session,
// a bot, and a scripted test all implement this, which is what keeps the
// engine free of I/O.
type ActionSource interface {
	RequestAction(ctx context.Context, v PlayerView) (Decision, error)
}

type Street int

const (
	Preflop Street = iota
	Flop
	Turn
	River
)

func (s Street) String() string {
	switch s {
	case Preflop:
		return "preflop"
	case Flop:
		return "flop"
	case Turn:
		return "turn"
	case River:
		return "river"
	}
	return "unknown"
}

// SeatInfo is the public state of one seat: everything every player at the
// table is entitled to see. It deliberately carries no hole cards.
type SeatInfo struct {
	Index      int
	Name       string
	Chips      int
	CurrentBet int
	TotalBet   int
	Folded     bool
	AllIn      bool
	IsButton   bool
}

// PlayerView is the redacted snapshot handed to one seat. Only that seat's
// own hole cards appear in it. This is a correctness requirement, not
// hardening: anything that reaches the client is visible to the player.
type PlayerView struct {
	Seat  int
	Hole  []deck.Card
	Board []deck.Card
	Seats []SeatInfo

	Street Street

	// Acting is the seat the table is waiting on, or SpectatorSeat when
	// no decision is pending.
	Acting int

	Pot        int
	CurrentBet int
	ToCall     int

	// MinRaiseTo and MaxRaiseTo bracket the legal Raise amounts. When
	// MinRaiseTo exceeds MaxRaiseTo the only aggressive move left is
	// shoving for MaxRaiseTo, which is always allowed.
	MinRaiseTo int
	MaxRaiseTo int

	// Deadline is when the seat on the clock runs out, and TurnLength is
	// how long they had. Both are the same for every viewer: the clock
	// belongs to the table, not to the player watching it, which is what
	// lets everyone see it tick down.
	Deadline   time.Time
	TurnLength time.Duration
}

// Legal reports whether an action is available in this spot, so a UI can
// grey out the rest instead of guessing.
func (v PlayerView) Legal(a Action) bool {
	switch a {
	case Fold:
		return true
	case Check:
		return v.ToCall == 0
	case Call:
		return v.ToCall > 0
	case Raise:
		return v.MaxRaiseTo > v.CurrentBet
	}
	return false
}
