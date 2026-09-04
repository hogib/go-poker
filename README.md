# ssh-holdem

Texas hold'em over SSH. Run the server, and anyone with an SSH client has a
seat — no install, no account, no client to build.

```
ssh -p 2222 yourname@your-host
```

## Running it

```sh
make run          # build and deal on 2222
make check        # gofmt, go vet, and every test under -race
make help         # everything else
```

Or by hand:

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

Connecting asks what to call you, prefilled with a poker handle. Your ssh
username is never used at the table: it is usually a real login name, and
there is no reason to publish it to everyone in the room. Enter accepts the
handle, or type over it.

Then the lobby, not a hand in progress:

```
  ♠ ssh holdem ♥  no-limit texas hold'em, over ssh

  ╭───────────────────────────────────╮
  │                                   │
  │     ▸ Take a seat                 │
  │       Leave your seat             │
  │       Watch the table             │
  │       Table rules                 │
  │       How to play                 │
  │       Quit                        │
  │                                   │
  │   ──────────────────────────      │
  │   10/20 blinds · 2000 buy-in      │
  │   2 seated · 1 player watching    │
  │                                   │
  ╰───────────────────────────────────╯
    ↑↓ move  ·  enter select  ·  q quit
```

`↑↓` moves, `enter` selects, and options that do not apply are greyed
rather than hidden so the menu never reflows under the cursor. Up to nine
players share one table; hands deal automatically whenever at least two are
seated, and sitting down or standing up takes effect between hands.

The table is drawn as a table, with you always at the bottom:

```
                             Frank         Erin
                             2850          2480
                             bets 60

   Gina      ╭──────────────────────────────────────╮   Dave
   3220      │                                      │   2110
   folded    │                 turn                 │   all in 940
             │             2♣ 7♦ T♠ A♥              │
             │               pot 320                │
   Hank      │                                      │   Carol   D
   3590      ╰──────────────────────────────────────╯   1740
   all in                                               folded

                             ╭──────────────╮
                  Iris       │you           │       Bob
                  3960       │1000          │       1370
                             │bets 60       │       bets 60
                             │███████░░░ 21s│
                             ╰──────────────╯
  you      A♠ K♥
```

Whoever the table is waiting on gets a box drawn round them with their shot
clock draining inside it, so it is obvious whose action it is without
reading anything. Seats are placed clockwise from you, so the layout means
the same thing to every player.

`v` switches to a compact list, which is also what a terminal too small for
the oval falls back to on its own.

At the table:

| Key   | Action                                                      |
| ----- | ----------------------------------------------------------- |
| `f`   | fold                                                        |
| `c`   | check, or call the outstanding bet                          |
| `r`   | raise — type an amount, `enter` to confirm, `esc` to cancel |
| `a`   | shove all in                                                |
| `r`   | buy in again, when you have been knocked out                |
| `v`   | switch between the table and the compact list               |
| `esc` | back to the menu                                            |
| `q`   | quit                                                        |

Your remaining time counts down beside the prompt as well, turning red under
five seconds. Let it run out and you check if you can and fold if you
cannot. Being put on the clock pulls you back to the table from wherever you
are, so nobody times out on a help screen.

Names are yours for as long as you are connected: twelve characters, no
duplicates, and non-printable characters are stripped — a name is written
into every other player's terminal, so an escape sequence in one would let a
player scribble on everyone else's screen.

Everything scales to the terminal. Panels take the width they are given, the
hand log trims on a short screen, and the key hints shed their least useful
entries rather than wrapping.

Disconnecting mid-hand folds you immediately rather than holding everyone
else for the rest of the clock. Reconnect with the same SSH key and you get
your name, your seat and your stack back — unless somebody claimed the name
while you were away, in which case you are numbered like anyone else rather
than there being two of you. Standing up banks your chips the same way, so
you can drop to the rail and sit back down with what you had.

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

| Package  | What it does                                                        |
| -------- | ------------------------------------------------------------------- |
| `deck`   | cards, shuffling, dealing, and a bit-packed five-card evaluator     |
| `player` | one seat's stack and per-street/per-hand contributions              |
| `game`   | the rules: blinds, betting rounds, streets, side pots, showdown     |
| `table`  | one concurrent table; owns the game, fans snapshots out to sessions |
| `tui`    | the Bubble Tea model that draws the lobby, the table and the felt   |
| `server` | the SSH listener that turns a connection into a player              |

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

Every package has tests, and they all run clean under `-race`:

| Package  | What its tests cover                                                                                                    |
| -------- | ----------------------------------------------------------------------------------------------------------------------- |
| `deck`   | all nine hand categories ranked against each other, tie-breaks, evaluator purity, best-five-of-seven, dealing and burns |
| `player` | betting, clamping to the stack, all-in, reset discipline                                                                |
| `game`   | blinds, betting rounds, side pots, showdown, the dead button, plus the fuzz gate                                        |
| `table`  | concurrency, redaction, disconnect, reconnect, banking, the lobby                                                       |
| `tui`    | every key and screen, the oval layout at every table and terminal size                                                  |
| `server` | identity, config, and real SSH connections end to end                                                                   |

The `tui` tests need no terminal: the model drives the table through a
`Controller` interface, so a fake records what each keypress asked for and
the tests assert on `View()` output. The `server` tests open a real
listener, connect over SSH with a real key, request a PTY, and check that
the lobby renders and that a keypress reaches the program.

Fixes are checked against the behaviour they replaced. Reverting the chip
banking, the dead button ring, the seat-eligibility rule or the refresh on
publish each makes a specific test fail with a specific message.

Several of the bugs these tests pin were found by running the thing rather
than by reasoning about it — reconnects handing out a fresh buy-in, a lobby
stuck on "connecting...", and keystrokes typed during connection setup being
swallowed. Each has a test now, but none of them had one first.

## Special Thanks

This project was made possible by the generous contributions of Mr Ali Tortop.
