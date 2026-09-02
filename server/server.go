// Package server exposes a poker table over SSH. Each connection becomes
// one Bubble Tea program driven by snapshots the table pushes.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"

	"ssh_holdem/table"
	"ssh_holdem/tui"
)

// Config is everything the listener needs. Table rules come from
// table.Config.
type Config struct {
	Host        string
	Port        int
	HostKeyPath string
	Table       table.Config
}

func (c Config) withDefaults() Config {
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 2222
	}
	if c.HostKeyPath == "" {
		c.HostKeyPath = ".ssh/ssh_holdem_ed25519"
	}
	return c
}

// Serve runs the table and the SSH listener until the process is
// interrupted.
func Serve(cfg Config) error {
	cfg = cfg.withDefaults()

	t := table.New(cfg.Table)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go t.Run(ctx)

	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	s, err := newServer(cfg, t)
	if err != nil {
		return err
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("dealing on ssh://%s -- connect with: ssh -p %d %s", addr, cfg.Port, cfg.Host)

	go func() {
		if err := s.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
			log.Println("server error:", err)
			done <- syscall.SIGTERM
		}
	}()

	<-done

	log.Println("shutting down")
	cancel()

	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	if err := s.Shutdown(shutdownCtx); err != nil && err != ssh.ErrServerClosed {
		return err
	}
	return nil
}

// newServer builds the listener. It is separate from Serve so a test can
// drive a real server without also taking over signal handling.
func newServer(cfg Config, t *table.Table) (*ssh.Server, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		// Any key is welcome; the fingerprint is only used as a stable
		// identity so a player keeps their seat across reconnects.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		wish.WithMiddleware(
			// Middleware is applied so that the last entry runs first, so
			// this list reads inside-out: the app, then the PTY check,
			// then logging around everything.
			bm.MiddlewareWithProgramHandler(handler(t), termenv.ANSI256),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create the ssh server: %w", err)
	}

	return s, nil
}

// handler turns one SSH session into a running Bubble Tea program joined
// to the table.
func handler(t *table.Table) bm.ProgramHandler {
	return func(sess ssh.Session) *tea.Program {
		session, publish := join(t, sess)

		styles := tui.NewStyles(newRenderer(sess))
		model := tui.New(t, session.ID, session.Name, styles)

		opts := append([]tea.ProgramOption{tea.WithAltScreen()}, bm.MakeOptions(sess)...)
		program := tea.NewProgram(model, opts...)
		publish(program)

		// Leaving is what releases the seat and unblocks any turn this
		// player is holding, so it must happen however the program ends.
		go func() {
			<-sess.Context().Done()
			t.Leave(session)
		}()

		return program
	}
}

// join connects a session to the table and returns the function that
// starts delivery once the program exists.
//
// The table needs somewhere to push messages at join time, but the
// program cannot be built until Join hands back the session. The closure
// reads through an atomic holder because the table's delivery goroutine
// and this one would otherwise race on it.
//
// Anything the table pushes before the program is published has nowhere
// to go, so publishing is also when the session asks for its state
// again. An idle table broadcasts nothing until something changes, so a
// client that did not ask would sit on an empty lobby until the next
// hand.
func join(t *table.Table, sess ssh.Session) (*table.Session, func(*tea.Program)) {
	var holder atomic.Pointer[tea.Program]

	session := t.Join(identify(sess), displayName(sess), func(msg any) {
		if p := holder.Load(); p != nil {
			p.Send(msg)
		}
	})

	publish := func(p *tea.Program) {
		holder.Store(p)
		t.Refresh(session.ID)
	}

	return session, publish
}

// newRenderer builds the lipgloss renderer for a session without asking
// the terminal anything.
//
// wish's MakeRenderer queries the terminal for its background colour and
// device attributes, and reads the replies off the session's own input.
// Whatever else the client has already sent is read in the same batch and
// discarded -- a keypress typed during connection setup, or input ssh had
// buffered, never reaches the program. The palette here is fixed rather
// than adaptive, so the background colour was never used; not asking for
// it keeps those keystrokes.
func newRenderer(sess ssh.Session) *lipgloss.Renderer {
	pty, _, ok := sess.Pty()
	if !ok || pty.Term == "" || pty.Term == "dumb" {
		return lipgloss.NewRenderer(sess, termenv.WithProfile(termenv.Ascii))
	}

	env := sshEnviron(append(sess.Environ(), "TERM="+pty.Term))

	return lipgloss.NewRenderer(sess,
		termenv.WithEnvironment(env),
		termenv.WithUnsafe(),
		termenv.WithColorCache(true),
	)
}

// sshEnviron lets termenv read the client's environment rather than the
// server process's, which is what decides the colour profile.
type sshEnviron []string

func (e sshEnviron) Environ() []string { return e }

func (e sshEnviron) Getenv(key string) string {
	prefix := key + "="
	for _, entry := range e {
		if strings.HasPrefix(entry, prefix) {
			return entry[len(prefix):]
		}
	}
	return ""
}

// identify returns a stable id for the connecting client. The public key
// fingerprint gives persistent seats with no login system; a client with
// no key falls back to its address, which is good enough to keep one
// terminal's seat for the length of a sitting.
func identify(sess ssh.Session) string {
	if key := sess.PublicKey(); key != nil {
		sum := sha256.Sum256(key.Marshal())
		return "key:" + base64.RawStdEncoding.EncodeToString(sum[:])
	}
	return "addr:" + sess.RemoteAddr().String()
}

func displayName(sess ssh.Session) string {
	if name := sess.User(); name != "" {
		return name
	}
	return "player"
}
