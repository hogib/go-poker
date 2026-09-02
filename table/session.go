// Package table runs a poker table for many concurrent players. It owns
// the engine's *game.Game on a single goroutine; sessions reach it only
// through channels, and receive redacted snapshots pushed back to them.
package table

import (
	"sync"

	"ssh_holdem/game"
	"ssh_holdem/player"
)

// outboxDepth is how many undelivered messages a session may queue before
// the oldest are dropped. The engine must never block on a slow terminal,
// and a client behind by several frames only cares about the newest state.
const outboxDepth = 8

// Session is one connected player. The exported fields are read-only once
// the session is created.
type Session struct {
	ID   string // stable across reconnects; the SSH key fingerprint
	Name string

	// Player is the seat's chip stack, and is the table's handle on it.
	// A pointer rather than an index because indices shift whenever
	// someone leaves, and a stale index addresses the wrong seat.
	Player *player.Player

	notifyMu sync.Mutex
	notify   func(any)

	outbox chan any

	closeOnce sync.Once
	done      chan struct{}
}

func newSession(id, name string, chips int, notify func(any)) *Session {
	p := player.NewPlayer(name, chips)

	s := &Session{
		ID:     id,
		Name:   name,
		Player: &p,
		notify: notify,
		outbox: make(chan any, outboxDepth),
		done:   make(chan struct{}),
	}

	go s.pump()

	return s
}

// pump delivers queued messages on the session's own goroutine, so a
// client that stops reading cannot stall the table.
func (s *Session) pump() {
	for {
		select {
		case <-s.done:
			return
		case msg := <-s.outbox:
			s.deliver(msg)
		}
	}
}

// send queues a message, dropping the oldest if the client has fallen
// behind. State messages are snapshots, so the newest one is always the
// only one that matters.
func (s *Session) send(msg any) {
	for {
		select {
		case <-s.done:
			return
		case s.outbox <- msg:
			return
		default:
		}

		select {
		case <-s.outbox:
			// Dropped a stale frame; try again.
		default:
		}
	}
}

// Send queues a message for this session. It never blocks: a client that
// has stopped reading loses stale frames rather than holding the table.
func (s *Session) Send(msg any) { s.send(msg) }

// Done is closed when the session ends, so a blocked turn can give up
// immediately instead of waiting out the clock.
func (s *Session) Done() <-chan struct{} { return s.done }

func (s *Session) close() {
	s.closeOnce.Do(func() { close(s.done) })
}

// Messages pushed to sessions. A session receives exactly these types.
type (
	// StateMsg is the table as this session is allowed to see it.
	StateMsg struct {
		View game.PlayerView
	}

	// TurnMsg says this session is the one being waited on. The view in
	// the accompanying StateMsg carries the legal actions and deadline.
	TurnMsg struct {
		View game.PlayerView
	}

	// ResultMsg reports how a hand finished.
	ResultMsg struct {
		Result game.HandResult
		Seats  []game.SeatInfo
	}

	// InfoMsg is a line of table chatter: someone sat down, the table is
	// waiting for players, and so on.
	InfoMsg struct {
		Text string
	}
)

// Attach points the session at a new delivery function, which is how a
// client takes over once it is ready to draw.
func (s *Session) Attach(notify func(any)) { s.attach(notify) }

// attach points the session at a new delivery function, which is how a
// reconnect takes over an existing seat without disturbing a turn that is
// already in flight.
func (s *Session) attach(notify func(any)) {
	s.notifyMu.Lock()
	s.notify = notify
	s.notifyMu.Unlock()
}

func (s *Session) deliver(msg any) {
	s.notifyMu.Lock()
	notify := s.notify
	s.notifyMu.Unlock()

	if notify != nil {
		notify(msg)
	}
}

// LobbyMsg is what the menu needs: the house rules, and who is at the
// table as against watching from the rail.
type LobbyMsg struct {
	Rules    Config
	Seated   int
	Watching int

	YouAreSeated bool
	YourChips    int
}
