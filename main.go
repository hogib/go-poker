package main

import (
	"flag"
	"log"
	"time"

	"go_poker/server"
	"go_poker/table"
)

func main() {
	var (
		host        = flag.String("host", "0.0.0.0", "address to listen on")
		port        = flag.Int("port", 2222, "port to listen on")
		hostKey     = flag.String("host-key", ".ssh/go_poker_ed25519", "path to the server's ssh host key; generated if absent")
		smallBlind  = flag.Int("small-blind", 10, "small blind")
		bigBlind    = flag.Int("big-blind", 20, "big blind")
		buyIn       = flag.Int("buy-in", 2000, "starting stack")
		turnTimeout = flag.Duration("turn-timeout", 30*time.Second, "shot clock per decision")
		handDelay   = flag.Duration("hand-delay", 4*time.Second, "pause between hands")
	)
	flag.Parse()

	cfg := server.Config{
		Host:        *host,
		Port:        *port,
		HostKeyPath: *hostKey,
		Table: table.Config{
			SmallBlind:  *smallBlind,
			BigBlind:    *bigBlind,
			BuyIn:       *buyIn,
			TurnTimeout: *turnTimeout,
			HandDelay:   *handDelay,
		},
	}

	if err := server.Serve(cfg); err != nil {
		log.Fatal(err)
	}
}
