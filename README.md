# ssh-holdem

Texas hold'em over SSH. Run the server, and anyone with an SSH client has a
seat — no install, no account, no client to build.

```
ssh -p 2222 yourname@your-host
```

## Running it

```sh
go build -o ssh-holdem .
./ssh-holdem
```

The server generates its own host key on first run and prints the address to
connect to. Any public key is accepted; the fingerprint is used only as a
stable identity, which is what lets a player keep their seat and their chips
across a dropped connection.

```
Usage of ./ssh-holdem:
  -host string          address to listen on (default "0.0.0.0")
  -port int             port to listen on (default 2222)
  -host-key string      path to the server's ssh host key, generated if absent
                        (default ".ssh/ssh_holdem_ed25519")
  -small-blind int      small blind (default 10)
  -big-blind int        big blind (default 20)
  -buy-in int           starting stack (default 2000)
  -turn-timeout duration  shot clock per decision (default 30s)
  -hand-delay duration    pause between hands (default 4s)
```

A PTY is required, so `ssh -T` is rejected with a message rather than a blank
screen.

## Playing

Up to nine players share one table. Hands deal automatically whenever at
least two people are seated, and joining or leaving takes effect between
hands.

| Key | Action |
| --- | --- |
| `f` | fold |
| `c` | check, or call the outstanding bet |
| `r` | raise — type an amount, `enter` to confirm, `esc` to cancel |
| `a` | all-in |
| `r` | buy in again, when you have been knocked out |
| `q` | quit |

The seat the table is waiting on is marked with `<-`, and your remaining
time on the shot clock counts down beside the prompt. Let it run out and you
check if you can and fold if you cannot.

Disconnecting mid-hand folds you immediately rather than holding everyone
else for the rest of the clock. Reconnect with the same SSH key and you get
your stack back.

## The dead button

When someone busts or leaves, their seat stays in the ring for one orbit
rather than the table closing up around it. The big blind then advances by
exactly one seat every hand, which is the promise the dead button rule
exists to keep: nobody posts the big blind twice running, and nobody skips
it.

The consequence is that two positions can land on an empty seat. The small
blind goes dead — it is simply not posted, and no neighbour is charged for
it — and the button itself can sit on the vacated seat for a hand. The
vacated seat leaves the ring once the button has passed over it.

Compacting the seat list on every departure is what makes the naive version
unfair: the seats renumber underneath the button, so a player can end up on
it twice running, or pay the big blind twice, or skip it entirely.

## How it fits together

```
  ssh session ─┐
  ssh session ─┼─► table (one goroutine owns the game) ─► game ─► deck
  ssh session ─┘         ▲                    │                    │
                         │  decisions         │ redacted           │ player
                         └────────────────────┘ snapshots
```

| Package | What it does |
| --- | --- |
| `deck` | cards, shuffling, dealing, and a bit-packed five-card evaluator |
| `player` | one seat's stack and per-street/per-hand contributions |
| `game` | the rules: blinds, betting rounds, streets, side pots, showdown |
| `table` | one concurrent table; owns the game, fans snapshots out to sessions |
| `tui` | the Bubble Tea model that draws a table for one player |
| `server` | the SSH listener that turns a connection into a seated player |

Three decisions shape the rest:

**The engine does no I/O.** A seat's decisions come from an
`ActionSource`, which a human at a terminal, a bot, and a scripted test all
implement identically. That is what makes the engine testable without a
terminal and the server thin.

**Players never see the whole table.** `game.ViewFor(seat)` builds a
redacted snapshot carrying only that seat's hole cards, and it is the only
thing that crosses into a session. This is a correctness requirement rather
than hardening: anything the client receives is visible to the player, so a
full game state on the wire is a cheatable game regardless of what the
client chooses to draw.

**One goroutine owns the game.** Sessions reach it only through channels and
receive snapshots pushed back to them. Each session has a bounded outbox
that drops stale frames, so a player on a slow link can never stall the
table.

## Pot layering

Chips in the middle are tracked as a single running total. The side-pot
structure is derived at showdown, as a pure function of what each player
committed:

```
levels        = sorted distinct TotalBet > 0 across all contributors
layer.amount  = (level - previous) * count(players with TotalBet >= level)
layer.eligible = players still contesting with TotalBet >= level
```

Folded players count toward the amount — their chips stay in the pot — but
never toward eligibility. Chips at a level whose contributors have all
folded are dead money and roll into a live layer rather than vanishing.

Keeping the pot as one integer and deriving the layering means there is no
second source of truth to fall out of sync. What it does depend on is reset
discipline: `CurrentBet` clears between streets, `TotalBet` only between
hands. Clear `TotalBet` too eagerly and side pots silently collapse into
one — correctly, in every single-street test you could write.

## Tests

```sh
go test ./... -race
```

The engine's gate is a fuzz test that plays 300 random tables out to
bust-out with bots that shove, fold and raise at random — around 2,500
hands, 1,100 of them with multiple pot layers, up to six layers deep. It
asserts that chips are neither created nor destroyed and that the pot layers
account for exactly what was wagered. It found a real bug: dead money being
dropped.

Conservation invariants hold even when the wrong player wins, so eligibility
is pinned separately by a deterministic three-way test — a short stack
all-in with a set takes the main pot and is locked out of the side pot above
it.

The table's tests are headless and run under the race detector: a hand
played out by two sessions, snapshot redaction, disconnect, reconnect, chip
banking, and acting out of turn.
