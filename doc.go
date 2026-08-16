// Package goheos is a Go client library for the Denon/Marantz HEOS CLI
// protocol (v1.21), enabling control of HEOS-capable speakers and
// receivers over the local network.
//
// It is a port of the Python library pyheos (https://github.com/andrewsayre/pyheos)
// with an idiomatic Go API: context-aware commands, functional options,
// and event delivery over channels.
//
// A single TCP connection to one HEOS device controls the entire HEOS
// system (all players). Connect to a wired device when possible.
//
//	c, err := goheos.Connect(ctx, "192.168.1.10")
//	if err != nil { ... }
//	defer c.Disconnect()
//	players, err := c.GetPlayers(ctx)
package goheos
