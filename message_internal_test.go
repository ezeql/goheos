package goheos

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCommandURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cmd  heosCommand
		want string
	}{
		{
			name: "no parameters",
			cmd:  newCommand(cmdGetPlayers),
			want: "heos://player/get_players",
		},
		{
			name: "reverse alphabetical order",
			cmd: newCommand(cmdSetVolume).
				withIntParam(attrPlayerID, 1).
				withIntParam(attrLevel, 50),
			want: "heos://player/set_volume?pid=1&level=50",
		},
		{
			name: "quotes only ampersand equals percent",
			cmd: newCommand(cmdSignIn).
				withParam(attrUserName, "us&er=name%100"),
			want: "heos://system/sign_in?un=us%26er%3Dname%25100",
		},
		{
			name: "spaces and plus are not escaped",
			cmd: newCommand(cmdBrowseSearch).
				withParam(attrSearch, "two words+plus"),
			want: "heos://browse/search?search=two words+plus",
		},
		{
			name: "url goes last and unquoted",
			cmd: newCommand(cmdBrowsePlayStream).
				withParam(attrURL, "http://host/stream?a=1&b=2").
				withIntParam(attrPlayerID, 1),
			want: "heos://browse/play_stream?pid=1&url=http://host/stream?a=1&b=2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cmd.uri(false); got != tt.want {
				t.Errorf("uri(false) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandURIMasked(t *testing.T) {
	t.Parallel()
	cmd := newCommand(cmdSignIn).
		withParam(attrUserName, "user").
		withParam(attrPassword, "secret")
	got := cmd.uri(true)
	if strings.Contains(got, "secret") {
		t.Errorf("masked uri contains password: %q", got)
	}
	want := "heos://system/sign_in?un=user&pw=" + passwordMask
	if got != want {
		t.Errorf("uri(true) = %q, want %q", got, want)
	}
	if unmasked := cmd.uri(false); !strings.Contains(unmasked, "pw=secret") {
		t.Errorf("uri(false) should contain the password, got %q", unmasked)
	}
}

func TestDecodeMessage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"heos": {"result": "success", "command": "player/get_volume", "message": "pid=1&level=36.0"}}`)
	m, err := decodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.command != "player/get_volume" {
		t.Errorf("command = %q", m.command)
	}
	if !m.result {
		t.Error("result should be success")
	}
	if m.isEvent() || m.isUnderProcess() {
		t.Error("plain response misclassified")
	}
	level, err := m.intValue(attrLevel)
	if err != nil {
		t.Fatal(err)
	}
	if level != 36 {
		t.Errorf("level = %d, want 36 (decimal int parsing)", level)
	}
}

func TestDecodeMessageEvent(t *testing.T) {
	t.Parallel()
	// Events carry no result field: absent result means success.
	raw := []byte(`{"heos": {"command": "event/player_volume_changed", "message": "pid=1&level=10&mute=off"}}`)
	m, err := decodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !m.result {
		t.Error("absent result must be treated as success")
	}
	if !m.isEvent() {
		t.Error("event/* command must be classified as event")
	}
}

func TestDecodeMessageUnderProcess(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"heos": {"command": "player/clear_queue", "result": "success", "message": "command under process&pid=1"}}`)
	m, err := decodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !m.isUnderProcess() {
		t.Error("message must be classified as under process")
	}
	if v := m.message[attrPlayerID]; v != "1" {
		t.Errorf("pid = %q, want 1", v)
	}
}

func TestDecodeMessageBlankValuesAndUnescape(t *testing.T) {
	t.Parallel()
	m, err := decodeMessage([]byte(`{"heos": {"command": "system/check_account", "message": "signed_in&un=user%40example.com"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := m.message[attrSignedIn]; !ok || v != "" {
		t.Errorf("flag signed_in should be present with blank value, got %q ok=%v", v, ok)
	}
	if v := m.message[attrUserName]; v != "user@example.com" {
		t.Errorf("un = %q, want percent-decoded value", v)
	}
}

func TestMessageValueMissingKey(t *testing.T) {
	t.Parallel()
	m := &message{command: cmdBrowseBrowse, message: map[string]string{}}
	if _, err := m.value("missing_key"); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestDecodeFixtures(t *testing.T) {
	t.Parallel()
	// Every fixture without replacement tokens must decode cleanly.
	entries, err := os.ReadDir("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	decoded := 0
	for _, e := range entries {
		raw, err := os.ReadFile("testdata/fixtures/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		// Replacement tokens like {player_id} live inside JSON strings, so
		// every fixture is valid JSON and must decode.
		if _, err := decodeMessage(raw); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		decoded++
	}
	if decoded < 100 {
		t.Errorf("decoded %d fixtures, want >= 100", decoded)
	}
}

func TestCommandFailedErrorFromMessage(t *testing.T) {
	t.Parallel()
	m, err := decodeMessage([]byte(`{"heos": {"command": "system/sign_in", "result": "fail", "message": "eid=10&text=User not found"}}`))
	if err != nil {
		t.Fatal(err)
	}
	cf := newCommandFailedError(m)
	if cf.ErrorID != ErrorUserNotFound || cf.ErrorText != "User not found" {
		t.Errorf("unexpected error fields: %+v", cf)
	}
	if !errors.Is(cf, ErrAuthenticationFailure) {
		t.Error("eid=10 must classify as authentication failure")
	}
}

func TestCommandFailedErrorSystemError(t *testing.T) {
	t.Parallel()
	m, err := decodeMessage([]byte(`{"heos": {"command": "browse/browse", "result": "fail", "message": "eid=12&text=System error&syserrno=-1063"}}`))
	if err != nil {
		t.Fatal(err)
	}
	cf := newCommandFailedError(m)
	if cf.SystemErrorNumber != SystemErrorUserNotLoggedIn {
		t.Errorf("syserrno = %d", cf.SystemErrorNumber)
	}
	if cf.ErrorText != "System error -1063" {
		t.Errorf("syserrno must be appended to text, got %q", cf.ErrorText)
	}
	if !errors.Is(cf, ErrAuthenticationFailure) {
		t.Error("eid=12 with syserrno=-1063 must classify as authentication failure")
	}
}

func TestCommandFailedErrorNotAuthentication(t *testing.T) {
	t.Parallel()
	cf := &CommandFailedError{Command: cmdBrowseBrowse, ErrorID: ErrorInternal, ErrorText: "System Internal Error"}
	if errors.Is(cf, ErrAuthenticationFailure) {
		t.Error("eid=11 must not classify as authentication failure")
	}
}

func TestCommandErrorUnwrap(t *testing.T) {
	t.Parallel()
	ce := &CommandError{Command: cmdGetPlayers, Err: ErrDisconnected}
	if !errors.Is(ce, ErrDisconnected) {
		t.Error("CommandError must unwrap to its cause")
	}
}
