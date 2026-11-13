# goheos

[![CI](https://github.com/ezeql/goheos/actions/workflows/ci.yml/badge.svg)](https://github.com/ezeql/goheos/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ezeql/goheos.svg)](https://pkg.go.dev/github.com/ezeql/goheos)
[![Go Report Card](https://goreportcard.com/badge/github.com/ezeql/goheos)](https://goreportcard.com/report/github.com/ezeql/goheos)

A Go client library for the Denon/Marantz **HEOS CLI protocol** (v1.21),
controlling HEOS-capable speakers and receivers over the local network.

goheos is a full-parity port of the Python library
[pyheos](https://github.com/andrewsayre/pyheos) with an idiomatic Go API:
context-aware commands, functional options, errors that work with
`errors.Is`/`errors.As`, and event delivery over channels. It has **zero
runtime dependencies** (standard library only).

## Installation

```sh
go get github.com/ezeql/goheos
```

Requires Go 1.27 or later.

## Quick start

```go
ctx := context.Background()

c, err := goheos.Connect(ctx, "192.168.1.10",
    goheos.WithAutoReconnect(time.Second, 0),
)
if err != nil {
    log.Fatal(err)
}
defer c.Disconnect()

players, err := c.GetPlayers(ctx)
if err != nil {
    log.Fatal(err)
}
for _, p := range players {
    fmt.Println(p.Name(), p.State(), p.Volume())
}

// React to change events over a channel.
events, cancel := c.Subscribe()
defer cancel()
for e := range events {
    if pe, ok := e.(goheos.PlayerEvent); ok {
        fmt.Println("player", pe.PlayerID, pe.Command)
    }
}
```

See [examples/](examples/) for runnable programs.

A single TCP connection to one HEOS device controls the entire HEOS system
(all players). Connect to a device wired to your network when possible.

## Options

| Option | Default | Description |
|---|---|---|
| `WithTimeout(d)` | `15s` | Timeout for connecting and for command responses. |
| `WithoutEvents()` | events on | Disable change events entirely. |
| `WithChangedProgressEventsOnly()` | all progress | Only the first progress event after each media change. |
| `WithCredentials(user, pass)` | none | Sign in to the HEOS account on every (re)connection. |
| `WithHeartbeat(interval)` / `WithoutHeartbeat()` | `10s` | Keepalive ping after idle intervals. |
| `WithAutoReconnect(delay, maxAttempts)` | off | Reconnect with exponential backoff (`maxAttempts` 0 = unlimited). |
| `WithAutoFailover(hosts...)` | off | Rotate to backup hosts on failure; with no hosts, they are discovered from the system. |
| `WithLogger(l)` | discard | `*slog.Logger` for protocol traces. |

## Events

`Subscribe` returns a `<-chan Event` plus a cancel function. Concrete event
types: `ConnectedEvent`, `DisconnectedEvent`, `UserCredentialsInvalidEvent`,
`ControllerEvent` (players/groups/sources/user changed), `PlayerEvent` and
`GroupEvent`. The client's state (players, groups, sources) is always updated
*before* the event is delivered, and per-subscription queues are unbounded by
default so no event is lost — use `WithBoundedBuffer` to cap them. Filters:
`WithPlayerEvents(ids...)`, `WithGroupEvents(ids...)`, `WithEventFilter(f)`.

## Errors

```go
err := c.SignIn(ctx, user, pass)
if errors.Is(err, goheos.ErrAuthenticationFailure) { ... }

var failed *goheos.CommandFailedError // the device replied "fail"
if errors.As(err, &failed) {
    fmt.Println(failed.ErrorID, failed.ErrorText)
}
if errors.Is(err, context.DeadlineExceeded) { ... } // command timed out
```

Client-side validation failures wrap `goheos.ErrInvalidArgument`; connection
problems wrap `goheos.ErrNotConnected` / `goheos.ErrDisconnected`.

## Mapping from pyheos

| pyheos | goheos |
|---|---|
| `Heos.create_and_connect(host, ...)` | `goheos.Connect(ctx, host, opts...)` |
| `Heos.validate_connection(host)` | `goheos.ValidateConnection(ctx, host)` |
| `heos.players` / `get_players()` | `c.Players()` / `c.GetPlayers(ctx)` |
| `load_players()` | `c.LoadPlayers(ctx)` |
| `player_set_volume(pid, level)` | `c.SetVolume(ctx, pid, level)` or `p.SetVolume(ctx, level)` |
| `get_groups(refresh=True)` | `c.RefreshGroups(ctx)` |
| `set_group([1,2,3])` | `c.SetGroup(ctx, []int{1,2,3})` |
| `browse(sid, cid, start, end)` | `c.Browse(ctx, sid, goheos.WithContainer(cid), goheos.WithRange(start, end))` |
| `get_favorites()` | `c.GetFavorites(ctx)` (1-based map, matches `PlayPresetStation`) |
| `play_media(pid, media)` | `c.PlayMedia(ctx, pid, media, criteria)` |
| `set_service_option(option_id, ...)` | `c.SetServiceOption(ctx, option, goheos.OptionSourceID(1), ...)` |
| `dispatcher.connect(...)` callbacks | `c.Subscribe(opts...)` channel |
| `CommandAuthenticationError` | `errors.Is(err, goheos.ErrAuthenticationFailure)` |
| `pyheos.util.mediauri` | `github.com/ezeql/goheos/mediauri` (byte-compatible URIs) |

## Development

```sh
make lint   # golangci-lint v2 (incl. gofumpt formatting)
make race   # tests with the race detector
make cover  # coverage profile
make vuln   # govulncheck
```

The test suite runs against a fake HEOS device (a real TCP server speaking
the CLI protocol from recorded fixtures) — the same approach and the same
103 protocol fixtures as pyheos. Golden files regenerate with
`go test -update ./...`.

## License

[Apache-2.0](LICENSE). Port of [pyheos](https://github.com/andrewsayre/pyheos)
by Andrew Sayre; see [NOTICE](NOTICE).
