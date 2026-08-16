package heostest

import (
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Matcher matches an incoming command (optionally with exact arguments) and
// replies with one or more fixture responses. Response behavior can be
// modified fluently (delay, replay, event side effects), replacing pyheos'
// CommandModifier context manager.
type Matcher struct {
	command  string
	args     map[string]string // nil accepts any arguments
	fixtures []string

	mu        sync.Mutex
	callCount int
	delay     time.Duration
	replay    int
	preEvents []string
}

// WithDelay delays each response, e.g. to force a command timeout.
func (m *Matcher) WithDelay(d time.Duration) *Matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = d
	return m
}

// WithReplay writes the response n times, simulating duplicated responses.
func (m *Matcher) WithReplay(n int) *Matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replay = n
	return m
}

// WithPreEvents sends the given event fixtures over the event channel right
// before responding, simulating events racing a command response.
func (m *Matcher) WithPreEvents(fixtures ...string) *Matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preEvents = fixtures
	return m
}

// matches reports whether the command and arguments match, incrementing the
// call count when increment is set.
func (m *Matcher) matches(command string, args map[string]string, increment bool) bool {
	if m.command != command {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.args != nil && args != nil && !mapsEqual(m.args, args) {
		return false
	}
	if increment {
		m.callCount++
	}
	return true
}

// responses renders the matcher's fixtures with the token substitutions
// pyheos applies: selected query values, the command name, and the full
// parameter list.
func (m *Matcher) responses(query map[string]string) []string {
	tokens := map[string]string{
		"pid":     "{player_id}",
		"state":   "{state}",
		"level":   "{level}",
		"options": "{options}",
	}
	out := make([]string, 0, len(m.fixtures))
	for _, fixture := range m.fixtures {
		body := Fixture(fixture)
		for key, token := range tokens {
			if value, ok := query[key]; ok {
				body = strings.ReplaceAll(body, token, url.QueryEscape(value))
			}
		}
		body = strings.ReplaceAll(body, "{command}", m.command)
		body = strings.ReplaceAll(body, "{parameters}", encodeQuery(query))
		out = append(out, body)
	}
	return out
}

// CallCount returns how many times the matcher was hit.
func (m *Matcher) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// AssertCalled fails the test if the matcher was never hit.
func (m *Matcher) AssertCalled(t *testing.T) {
	t.Helper()
	if m.CallCount() == 0 {
		t.Errorf("command %s was not called with arguments %v", m.command, m.args)
	}
}

func (m *Matcher) modifiers() (delay time.Duration, replay int, preEvents []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	replay = m.replay
	if replay == 0 {
		replay = 1
	}
	return m.delay, replay, m.preEvents
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func encodeQuery(query map[string]string) string {
	values := url.Values{}
	for k, v := range query {
		values.Set(k, v)
	}
	return values.Encode()
}
