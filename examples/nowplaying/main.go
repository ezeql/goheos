// Command nowplaying connects to a HEOS device, lists the players in the
// system and prints what each one is playing.
//
// Usage: go run ./examples/nowplaying <host>
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ezeql/goheos"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <host>", os.Args[0])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := goheos.Connect(ctx, os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = c.Disconnect() }()

	players, err := c.GetPlayers(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range players {
		np := p.NowPlaying()
		fmt.Printf("%s (%s) [%s]\n", p.Name(), p.Model(), p.State())
		if np.Song != "" {
			fmt.Printf("  ♪ %s — %s (%s)\n", np.Song, np.Artist, np.Album)
		} else if np.Station != "" {
			fmt.Printf("  ♪ %s\n", np.Station)
		}
	}
}
