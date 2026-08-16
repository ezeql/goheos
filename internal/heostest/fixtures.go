// Package heostest provides a fake HEOS device: a real TCP server speaking
// the HEOS CLI protocol from recorded fixtures, used to test the client
// without hardware. It is a port of pyheos' MockHeosDevice.
package heostest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// fixturesDir resolves testdata/fixtures relative to this source file so the
// loader works regardless of the working directory of the test binary.
var fixturesDir = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("heostest: cannot resolve source file location")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures")
}()

var (
	fixtureCacheMu sync.Mutex
	fixtureCache   = map[string]string{}
)

// Fixture returns the raw content of testdata/fixtures/<name>.json. It panics
// on a missing fixture, which is a bug in the test itself.
func Fixture(name string) string {
	fixtureCacheMu.Lock()
	defer fixtureCacheMu.Unlock()
	if s, ok := fixtureCache[name]; ok {
		return s
	}
	raw, err := os.ReadFile(filepath.Join(fixturesDir, name+".json"))
	if err != nil {
		panic("heostest: fixture " + name + ": " + err.Error())
	}
	// Normalize CRLF like Python's text-mode open does: a stray CRLF inside a
	// fixture would break the protocol framing.
	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	fixtureCache[name] = s
	return s
}

// FixtureExists reports whether a fixture file is present.
func FixtureExists(name string) bool {
	_, err := os.Stat(filepath.Join(fixturesDir, name+".json"))
	return err == nil
}

// replaceTokens substitutes {key} tokens in a fixture body.
func replaceTokens(body string, replacements map[string]string) string {
	for key, value := range replacements {
		body = strings.ReplaceAll(body, "{"+key+"}", value)
	}
	return body
}
