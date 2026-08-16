// Command events connects to a HEOS device with auto reconnect enabled and
// prints every change event until interrupted.
//
// Usage: go run ./examples/events <host>
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/ezeql/goheos"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <host>", os.Args[0])
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	c, err := goheos.Connect(ctx, os.Args[1],
		goheos.WithAutoReconnect(time.Second, 0))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = c.Disconnect() }()

	// Load players so player events resolve against live state.
	if _, err := c.GetPlayers(ctx); err != nil {
		log.Fatal(err)
	}

	events, cancel := c.Subscribe()
	defer cancel()

	fmt.Println("listening for events, press Ctrl-C to exit")
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			switch ev := e.(type) {
			case goheos.ConnectedEvent:
				fmt.Println("connected to", c.Host())
			case goheos.DisconnectedEvent:
				fmt.Printf("disconnected (error: %v)\n", ev.FromError)
			case goheos.PlayerEvent:
				p := c.Players()[ev.PlayerID]
				if p == nil {
					continue
				}
				fmt.Printf("player %s: %s (state=%s volume=%d)\n",
					p.Name(), ev.Command, p.State(), p.Volume())
			case goheos.GroupEvent:
				fmt.Printf("group %d: %s\n", ev.GroupID, ev.Command)
			case goheos.ControllerEvent:
				fmt.Println("controller:", ev.Command)
			case goheos.UserCredentialsInvalidEvent:
				fmt.Println("credentials are no longer valid")
			}
		}
	}
}
