package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"

	"ssh_holdem/table"
)

func TestConfigDefaults(t *testing.T) {
	got := Config{}.withDefaults()

	if got.Host != "0.0.0.0" {
		t.Errorf("expected a default host, got %q", got.Host)
	}
	if got.Port != 2222 {
		t.Errorf("expected the default port, got %d", got.Port)
	}
	if got.HostKeyPath == "" {
		t.Error("a host key path is required; the server generates the key itself")
	}
}

func TestConfigDefaultsLeaveSuppliedValuesAlone(t *testing.T) {
	given := Config{
		Host:        "127.0.0.1",
		Port:        4242,
		HostKeyPath: "/tmp/key",
		Table:       table.Config{SmallBlind: 1, BigBlind: 2},
	}

	got := given.withDefaults()

	if got.Host != given.Host || got.Port != given.Port || got.HostKeyPath != given.HostKeyPath {
		t.Errorf("defaults overwrote supplied values: %+v", got)
	}
	if got.Table.SmallBlind != 1 {
		t.Errorf("the table's own rules should be passed through, got %+v", got.Table)
	}
}

// fakeSession is the slice of ssh.Session the handler's helpers touch.
type fakeSession struct {
	ssh.Session

	user string
	key  ssh.PublicKey
	addr net.Addr
}

func (s fakeSession) User() string             { return s.user }
func (s fakeSession) PublicKey() ssh.PublicKey { return s.key }
func (s fakeSession) RemoteAddr() net.Addr     { return s.addr }

func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	signer, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("wrapping the key: %v", err)
	}
	return signer
}

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

// The key fingerprint is what gives a player a persistent seat, so the
// same key must produce the same id every time and different keys must
// not collide.
func TestIdentifyIsStableForOneKey(t *testing.T) {
	key := testKey(t)

	first := identify(fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.1:1")})
	second := identify(fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.2:2")})

	if first != second {
		t.Errorf("the same key from a different address should be the same player:\n%s\n%s",
			first, second)
	}
	if !strings.HasPrefix(first, "key:") {
		t.Errorf("a key-based id should say so, got %q", first)
	}
}

func TestIdentifySeparatesDifferentKeys(t *testing.T) {
	a := identify(fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")})
	b := identify(fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")})

	if a == b {
		t.Error("two different keys must not share an identity")
	}
}

// A client with no key still gets an identity for the length of its
// sitting, and one that cannot be confused with a key-based one.
func TestIdentifyFallsBackToTheAddress(t *testing.T) {
	got := identify(fakeSession{user: "alice", addr: fakeAddr("10.0.0.7:2222")})

	if !strings.HasPrefix(got, "addr:") {
		t.Errorf("expected an address-based id, got %q", got)
	}
	if strings.Contains(got, "key:") {
		t.Errorf("an address id must not look like a key id, got %q", got)
	}
}

// Two keyless clients from different addresses are different players;
// from the same address they are the same one.
func TestIdentifyDistinguishesAddresses(t *testing.T) {
	a := identify(fakeSession{addr: fakeAddr("10.0.0.1:1")})
	b := identify(fakeSession{addr: fakeAddr("10.0.0.2:1")})
	again := identify(fakeSession{addr: fakeAddr("10.0.0.1:1")})

	if a == b {
		t.Error("different addresses should be different players")
	}
	if a != again {
		t.Error("the same address should be the same player")
	}
}

// The ssh username is someone's real login name. It identifies them for
// seating purposes through the key fingerprint, and must not end up on
// the table where everyone can read it.
func TestTheSSHUsernameNeverReachesTheTable(t *testing.T) {
	tbl := testTable()

	for _, user := range []string{"oguz", "root", "j.smith"} {
		sess := fakeSession{user: user, key: testKey(t), addr: fakeAddr("10.0.0.1:1")}
		session, _ := join(tbl, sess)

		if got := session.Name(); strings.EqualFold(got, user) {
			t.Errorf("the ssh username %q was published as the table name %q", user, got)
		}
		if session.Name() == "" {
			t.Error("a player still needs something to be called")
		}
	}
}

func testTable() *table.Table {
	return table.New(table.Config{
		SmallBlind:  10,
		BigBlind:    20,
		BuyIn:       1000,
		TurnTimeout: time.Second,
		HandDelay:   time.Millisecond,
	})
}

// join must register the session against its key identity, so a
// reconnect finds the same seat.
func TestJoinRegistersTheSessionUnderItsKeyIdentity(t *testing.T) {
	tbl := testTable()
	sess := fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")}

	session, publish := join(tbl, sess)

	if session == nil {
		t.Fatal("join returned no session")
	}
	if publish == nil {
		t.Fatal("join returned no way to publish the program")
	}
	if session.Name() == "" {
		t.Error("a player needs a name to show at the table")
	}
	if got := tbl.Session(identify(sess)); got != session {
		t.Error("the session should be registered under its key identity")
	}
}

// Connecting must not take a seat: a new arrival lands in the lobby and
// chooses for themselves.
func TestJoinDoesNotSeatOnConnect(t *testing.T) {
	tbl := testTable()
	sess := fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")}

	join(tbl, sess)

	if seated := tbl.SeatedCount(); seated != 0 {
		t.Errorf("connecting should take no seat, got %d seated", seated)
	}
	if watching := tbl.Watchers(); watching != 1 {
		t.Errorf("expected one watcher in the lobby, got %d", watching)
	}
}

// Reconnecting with the same key must reattach rather than opening a
// second identity for the same player.
func TestJoinReattachesTheSameKey(t *testing.T) {
	tbl := testTable()
	key := testKey(t)

	first, _ := join(tbl, fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.1:1")})
	second, _ := join(tbl, fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.9:9")})

	if first != second {
		t.Error("the same key from a new address should reattach to the same session")
	}
	if watching := tbl.Watchers(); watching != 1 {
		t.Errorf("a reconnect should not add a second watcher, got %d", watching)
	}
}

// Nothing may be pushed at a program that does not exist yet, and
// everything must reach it once it does.
func TestPublishGatesDeliveryUntilTheProgramExists(t *testing.T) {
	tbl := testTable()
	sess := fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")}

	// join sends the lobby state immediately, before any program is
	// published. That must not panic or block.
	session, publish := join(tbl, sess)

	done := make(chan struct{})
	go func() {
		defer close(done)
		session.Send(table.InfoMsg{Text: "before the program"})
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sending before the program exists blocked")
	}

	// Publishing a real program must be accepted.
	publish(tea.NewProgram(stubModel{}))
}

type stubModel struct{}

func (stubModel) Init() tea.Cmd                         { return nil }
func (m stubModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (stubModel) View() string                          { return "" }

// newServer must produce a listener configured with the middleware, and
// must not need a pre-existing host key.
func TestNewServerGeneratesItsOwnHostKey(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	s, err := newServer(cfg, testTable())
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	if s.Handler == nil {
		t.Error("the server has no handler, so a connection would do nothing")
	}
	if len(s.HostSigners) == 0 {
		t.Error("the server should have generated a host key")
	}

	if _, err := os.Stat(cfg.HostKeyPath); err != nil {
		t.Errorf("the host key should have been written to %s: %v", cfg.HostKeyPath, err)
	}
}

// End to end over a real SSH connection: the listener accepts a key,
// allocates a PTY, and the client is dealt a lobby to look at.
func TestServerServesTheLobbyOverSSH(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	tbl := testTable()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	srv, err := newServer(cfg, tbl)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go srv.Serve(listener)
	defer srv.Close()

	client := dialAsPlayer(t, listener.Addr().String())
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer sess.Close()

	if err := sess.RequestPty("xterm-256color", 40, 100, gossh.TerminalModes{}); err != nil {
		t.Fatalf("requesting a pty: %v", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if _, err := sess.StdinPipe(); err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("starting the shell: %v", err)
	}

	// The read has to happen off the test goroutine: an ssh channel read
	// blocks until bytes arrive, so a deadline checked around it would
	// never be reached.
	var (
		mu   sync.Mutex
		seen strings.Builder
	)

	found := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				mu.Lock()
				seen.Write(buf[:n])
				done := strings.Contains(stripANSI(seen.String()), "What should we call you?")
				mu.Unlock()

				if done {
					close(found)
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-found:
		return
	case <-time.After(15 * time.Second):
		mu.Lock()
		got := stripANSI(seen.String())
		mu.Unlock()
		t.Fatalf("the name prompt never rendered over ssh; saw %d bytes:\n%s", len(got), got)
	}
}

// A session with no PTY must be turned away with a message rather than
// left staring at a blank screen.
func TestServerRejectsSessionsWithoutAPty(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	srv, err := newServer(cfg, testTable())
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go srv.Serve(listener)
	defer srv.Close()

	client := dialAsPlayer(t, listener.Addr().String())
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer sess.Close()

	// No RequestPty: this is what `ssh -T` looks like.
	out, _ := sess.CombinedOutput("")

	if !strings.Contains(string(out), "PTY") {
		t.Errorf("expected a readable message about needing a terminal, got %q", out)
	}
}

func dialAsPlayer(t *testing.T, addr string) *gossh.Client {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating a client key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("wrapping the client key: %v", err)
	}

	client, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "alice",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatalf("dialling the table: %v", err)
	}
	return client
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansi.ReplaceAllString(s, "")
}

// Publishing the program must also pull the state the session missed.
// Without it a connecting player sees a lobby that says "connecting..."
// until the next hand -- which is what actually happened over ssh.
func TestPublishRefreshesTheLobby(t *testing.T) {
	tbl := testTable()
	sess := fakeSession{user: "alice", key: testKey(t), addr: fakeAddr("10.0.0.1:1")}

	session, publish := join(tbl, sess)

	// Let the queue Join filled drain into a program that does not exist
	// yet, which is where those messages are lost.
	time.Sleep(200 * time.Millisecond)

	// Now attach a recorder in the program's place and check that
	// nothing further arrives on its own.
	got := make(chan table.LobbyMsg, 8)
	session.Attach(func(msg any) {
		if lobby, ok := msg.(table.LobbyMsg); ok {
			select {
			case got <- lobby:
			default:
			}
		}
	})

	select {
	case lobby := <-got:
		t.Fatalf("an idle table should push nothing on its own, got %+v", lobby)
	case <-time.After(200 * time.Millisecond):
	}

	publish(tea.NewProgram(stubModel{}))

	select {
	case lobby := <-got:
		if lobby.Rules.BigBlind != 20 {
			t.Errorf("expected the house rules, got %+v", lobby.Rules)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publishing the program did not fetch the lobby state")
	}
}

// Keystrokes have to reach the program. This connects for real, waits
// for the lobby, presses enter, and checks that the table took a seat --
// the whole input path, which nothing else here exercises.
func TestKeystrokesReachTheProgramOverSSH(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	tbl := testTable()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	srv, err := newServer(cfg, tbl)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go srv.Serve(listener)
	defer srv.Close()

	client := dialAsPlayer(t, listener.Addr().String())
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer sess.Close()

	if err := sess.RequestPty("xterm-256color", 40, 100, gossh.TerminalModes{}); err != nil {
		t.Fatalf("requesting a pty: %v", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("starting the shell: %v", err)
	}

	lobbyUp := make(chan struct{})
	go func() {
		var seen strings.Builder
		buf := make([]byte, 4096)
		done := false
		for {
			n, err := stdout.Read(buf)
			if n > 0 && !done {
				seen.Write(buf[:n])
				if strings.Contains(stripANSI(seen.String()), "What should we call you?") {
					done = true
					close(lobbyUp)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-lobbyUp:
	case <-time.After(15 * time.Second):
		t.Fatal("the name prompt never rendered")
	}

	// The first enter accepts the prefilled name; the second takes the
	// seat the menu opens on.
	if _, err := stdin.Write([]byte("\r")); err != nil {
		t.Fatalf("writing the keypress: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := stdin.Write([]byte("\r")); err != nil {
		t.Fatalf("writing the keypress: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for tbl.SeatedCount() == 0 {
		select {
		case <-deadline:
			t.Fatal(`pressing enter on "What should we call you?" did not seat the player`)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Input sent during connection setup must survive.
//
// The renderer used to query the terminal for its background colour and
// read the replies off the session's own input, discarding whatever else
// arrived in the same batch. A player who typed while connecting -- or
// whose client sent buffered input -- lost those keystrokes silently.
func TestKeystrokesDuringConnectionSetupAreNotLost(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	tbl := testTable()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	srv, err := newServer(cfg, tbl)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go srv.Serve(listener)
	defer srv.Close()

	client := dialAsPlayer(t, listener.Addr().String())
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer sess.Close()

	if err := sess.RequestPty("xterm-256color", 40, 100, gossh.TerminalModes{}); err != nil {
		t.Fatalf("requesting a pty: %v", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("starting the shell: %v", err)
	}

	// Both keypresses go in immediately, before anything has been drawn:
	// the first accepts the prefilled name, the second takes a seat. If
	// either is swallowed, no seat is taken.
	if _, err := stdin.Write([]byte("\r\r")); err != nil {
		t.Fatalf("writing the keypresses: %v", err)
	}

	// Keep the output window from filling.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdout.Read(buf); err != nil {
				return
			}
		}
	}()

	deadline := time.After(15 * time.Second)
	for tbl.SeatedCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("a keypress sent during connection setup was swallowed")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// The name screen is prefilled from the ssh username on a first
// connection, and from the name the player chose on a reconnect.
func TestReconnectPrefillsTheChosenName(t *testing.T) {
	tbl := testTable()
	key := testKey(t)

	first, _ := join(tbl, fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.1:1")})
	handed := first.Name()
	if handed == "" {
		t.Fatal("a first connection should be given a handle")
	}

	if _, err := tbl.Rename(first.ID, "Ace"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	again, _ := join(tbl, fakeSession{user: "alice", key: key, addr: fakeAddr("10.0.0.2:2")})
	if again.Name() != "Ace" {
		t.Errorf("a reconnect should keep the chosen name, got %q (was handed %q)",
			again.Name(), handed)
	}
}

// The table view renders over a real connection at a real terminal size.
// The layout choice has unit tests; this checks the whole path, since a
// session whose pty reports no size falls back to the compact list and
// nothing else here would notice.
func TestTableViewRendersOverSSH(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		HostKeyPath: filepath.Join(dir, "hostkey"),
	}.withDefaults()

	tbl := testTable()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	srv, err := newServer(cfg, tbl)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go srv.Serve(listener)
	defer srv.Close()

	// Two players, so there is a hand to draw.
	screens := make([]<-chan string, 0, 2)
	for i := 0; i < 2; i++ {
		screens = append(screens, playOverSSH(t, listener.Addr().String()))
	}

	var lastScreen string
	deadline := time.After(20 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("the table never rendered a pot over ssh; last screen:\n%s", lastScreen)
		case screen := <-screens[0]:
			lastScreen = screen
			// The felt puts the pot in the middle of the table, well
			// clear of the left margin the compact list uses.
			for _, line := range strings.Split(screen, "\n") {
				if i := strings.Index(line, "pot "); i > 20 {
					return
				}
			}
		}
	}
}

// playOverSSH opens a session at a real terminal size, accepts the
// prefilled name, takes a seat, and keeps checking. It streams rendered
// screens back for the caller to assert on.
func playOverSSH(t *testing.T, addr string) <-chan string {
	t.Helper()

	client := dialAsPlayer(t, addr)
	t.Cleanup(func() { client.Close() })

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	if err := sess.RequestPty("xterm-256color", 42, 110, gossh.TerminalModes{}); err != nil {
		t.Fatalf("requesting a pty: %v", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("starting the shell: %v", err)
	}

	// Accept the name, take the seat, then check every hand along.
	go func() {
		time.Sleep(200 * time.Millisecond)
		stdin.Write([]byte("\r"))
		time.Sleep(200 * time.Millisecond)
		stdin.Write([]byte("\r"))
		for i := 0; i < 100; i++ {
			time.Sleep(150 * time.Millisecond)
			if _, err := stdin.Write([]byte("c")); err != nil {
				return
			}
		}
	}()

	// Chunks arrive split at arbitrary points, so what is streamed back
	// is everything drawn so far rather than the latest fragment.
	screens := make(chan string, 64)
	go func() {
		defer close(screens)

		var seen strings.Builder
		buf := make([]byte, 8192)

		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				seen.Write(buf[:n])
				select {
				case screens <- stripANSI(seen.String()):
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()

	return screens
}
