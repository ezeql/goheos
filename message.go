package goheos

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// quoteReplacer escapes values per the CLI specification: only '&', '=' and
// '%' are escaped. Standard URL escaping breaks searches and names.
var quoteReplacer = strings.NewReplacer("&", "%26", "=", "%3D", "%", "%25")

const passwordMask = "********"

// heosCommand is a command to be sent to the HEOS device.
type heosCommand struct {
	name       string
	parameters map[string]string
}

func newCommand(name string) heosCommand {
	return heosCommand{name: name, parameters: map[string]string{}}
}

func (c heosCommand) withParam(key, value string) heosCommand {
	c.parameters[key] = value
	return c
}

func (c heosCommand) withIntParam(key string, value int) heosCommand {
	return c.withParam(key, strconv.Itoa(value))
}

// uri renders the command as "heos://<name>?<query>". When mask is true,
// sensitive parameters are replaced for logging.
func (c heosCommand) uri(mask bool) string {
	if len(c.parameters) == 0 {
		return basePrefix + c.name
	}
	keys := make([]string, 0, len(c.parameters))
	for k := range c.parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Iterating sorted keys while prepending yields reverse-alphabetical
	// order, matching pyheos so protocol traces are comparable. The 'url'
	// parameter must go last and unquoted per the CLI spec.
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		value := c.parameters[k]
		if mask && k == attrPassword {
			value = passwordMask
		}
		if k == attrURL {
			pairs = append(pairs, k+"="+value)
		} else {
			pairs = append(pairs, "")
			copy(pairs[1:], pairs)
			pairs[0] = k + "=" + quoteReplacer.Replace(value)
		}
	}
	return basePrefix + c.name + "?" + strings.Join(pairs, "&")
}

// message is a decoded message received from a HEOS device.
type message struct {
	command string
	result  bool
	message map[string]string
	payload json.RawMessage
	options json.RawMessage
	raw     string
}

type envelope struct {
	Heos struct {
		Command string `json:"command"`
		Result  string `json:"result"`
		Message string `json:"message"`
	} `json:"heos"`
	Payload json.RawMessage `json:"payload"`
	Options json.RawMessage `json:"options"`
}

// decodeMessage parses a raw protocol line into a message.
func decodeMessage(raw []byte) (*message, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("goheos: decode message: %w", err)
	}
	return &message{
		command: env.Heos.Command,
		// An absent result means success; events carry no result field.
		result:  env.Heos.Result == "" || env.Heos.Result == valueSuccess,
		message: parseMessageParams(env.Heos.Message),
		payload: env.Payload,
		options: env.Options,
		raw:     string(raw),
	}, nil
}

// parseMessageParams parses the "message" field, a query string with blank
// values kept (flags like "signed_in" have no value). Undecodable escapes are
// preserved verbatim rather than failing, matching Python's parse_qsl.
func parseMessageParams(s string) map[string]string {
	params := map[string]string{}
	if s == "" {
		return params
	}
	for pair := range strings.SplitSeq(s, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		params[unescape(key)] = unescape(value)
	}
	return params
}

func unescape(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}

func (m *message) isUnderProcess() bool {
	_, ok := m.message[valueUnderProcess]
	return ok
}

func (m *message) isEvent() bool {
	return strings.HasPrefix(m.command, "event/")
}

func (m *message) value(key string) (string, error) {
	v, ok := m.message[key]
	if !ok {
		return "", fmt.Errorf("goheos: message %q: missing parameter %q", m.command, key)
	}
	return v, nil
}

func (m *message) floatValue(key string) (float64, error) {
	v, err := m.value(key)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("goheos: message %q: parameter %q: %w", m.command, key, err)
	}
	return f, nil
}

// intValue parses an integer parameter. Values may carry a decimal point, so
// they are parsed as float first.
func (m *message) intValue(key string) (int, error) {
	f, err := m.floatValue(key)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}
