package goheos

import (
	"context"
	"net"
	"net/netip"
	"sort"
)

// Host is a HEOS-capable device on the network. It is a snapshot and is not
// updated once created.
type Host struct {
	Name             string
	Model            string
	Serial           string
	Version          string
	IPAddress        string
	Network          NetworkType
	SupportedVersion bool
	// PreferredHost marks a wired host with a supported firmware version and
	// a valid IP address: the best kind of host to connect to.
	PreferredHost bool
}

func hostFromData(data payloadData) Host {
	version := data.str(attrVersion)
	ip := data.str(attrIPAddress)
	network := NetworkType(data.str(attrNetwork))
	if network == "" {
		network = NetworkUnknown
	}
	supported := isSupportedVersion(version)
	_, ipOK := parseAddr(ip)
	return Host{
		Name:             data.str(attrName),
		Model:            data.str(attrModel),
		Serial:           data.str(attrSerial),
		Version:          version,
		IPAddress:        ip,
		Network:          network,
		SupportedVersion: supported,
		PreferredHost:    network == NetworkWired && supported && ipOK,
	}
}

func parseAddr(ip string) (netip.Addr, bool) {
	if ip == "" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(ip)
	return addr, err == nil
}

// System is a snapshot of the HEOS system topology.
type System struct {
	SignedInUsername string
	IsSignedIn       bool
	// Host is the entry matching the host the client connected to, when
	// found among Hosts.
	Host                     *Host
	Hosts                    []Host
	PreferredHosts           []Host
	ConnectedToPreferredHost bool
}

func newSystem(signedInUsername, connectedHost string, hosts []Host) *System {
	s := &System{
		SignedInUsername: signedInUsername,
		IsSignedIn:       signedInUsername != "",
		Hosts:            hosts,
	}
	for i := range hosts {
		if hosts[i].IPAddress == connectedHost {
			host := hosts[i]
			s.Host = &host
			break
		}
	}
	for _, h := range hosts {
		if h.PreferredHost {
			s.PreferredHosts = append(s.PreferredHosts, h)
		}
	}
	// Preferred hosts order by (serial present first, serial, ip).
	sort.SliceStable(s.PreferredHosts, func(i, j int) bool {
		a, b := s.PreferredHosts[i], s.PreferredHosts[j]
		if (a.Serial == "") != (b.Serial == "") {
			return b.Serial == ""
		}
		if a.Serial != b.Serial {
			return a.Serial < b.Serial
		}
		return lessAddr(a.IPAddress, b.IPAddress)
	})
	// All hosts order by (ip present first, ip).
	sort.SliceStable(s.Hosts, func(i, j int) bool {
		a, b := s.Hosts[i], s.Hosts[j]
		if (a.IPAddress == "") != (b.IPAddress == "") {
			return b.IPAddress == ""
		}
		return lessAddr(a.IPAddress, b.IPAddress)
	})
	if s.Host != nil {
		for _, h := range s.PreferredHosts {
			if h == *s.Host {
				s.ConnectedToPreferredHost = true
				break
			}
		}
	}
	return s
}

// lessAddr orders by numeric IP with unparsable addresses last, mirroring
// pyheos' (parse_failed, ip_address) sort keys.
func lessAddr(a, b string) bool {
	addrA, okA := parseAddr(a)
	addrB, okB := parseAddr(b)
	if okA != okB {
		return okA
	}
	if !okA {
		return false
	}
	return addrA.Compare(addrB) < 0
}

// IPAddresses returns the host IP addresses ordered as pyheos does for the
// failover list: sorted by (preferred flag ascending, ip).
func (s *System) IPAddresses() []string {
	hosts := make([]Host, 0, len(s.Hosts))
	for _, h := range s.Hosts {
		if h.IPAddress != "" {
			hosts = append(hosts, h)
		}
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		a, b := hosts[i], hosts[j]
		if a.PreferredHost != b.PreferredHost {
			return !a.PreferredHost
		}
		return lessAddr(a.IPAddress, b.IPAddress)
	})
	addresses := make([]string, len(hosts))
	for i, h := range hosts {
		addresses[i] = h.IPAddress
	}
	return addresses
}

// RegisterForChangeEvents enables or disables change events from the device.
// It is re-sent automatically on every (re)connection.
func (c *Client) RegisterForChangeEvents(ctx context.Context, enable bool) error {
	_, err := c.conn.command(ctx, newCommand(cmdRegisterForChangeEvents).
		withParam(attrEnable, onOff(enable)))
	return err
}

// CheckAccount returns the signed-in username, if any.
func (c *Client) CheckAccount(ctx context.Context) (username string, signedIn bool, err error) {
	resp, err := c.conn.command(ctx, newCommand(cmdAccountCheck))
	if err != nil {
		return "", false, err
	}
	if _, ok := resp.message[attrSignedIn]; ok {
		username = resp.message[attrUserName]
	}
	c.setSignedInUsername(username)
	return username, username != "", nil
}

// SignIn signs in to the HEOS account. On success the credentials are stored
// and reused to sign in automatically on reconnection.
func (c *Client) SignIn(ctx context.Context, username, password string) error {
	return c.signIn(ctx, username, password, true)
}

func (c *Client) signIn(ctx context.Context, username, password string, updateCredential bool) error {
	resp, err := c.conn.command(ctx, newCommand(cmdSignIn).
		withParam(attrUserName, username).
		withParam(attrPassword, password))
	if err != nil {
		return err
	}
	c.setSignedInUsername(resp.message[attrUserName])
	if updateCredential {
		c.setCredentials(&Credentials{Username: username, Password: password})
	}
	return nil
}

// SignOut signs out of the HEOS account and clears the stored credentials.
func (c *Client) SignOut(ctx context.Context) error {
	_, err := c.conn.command(ctx, newCommand(cmdSignOut))
	if err != nil {
		return err
	}
	c.setSignedInUsername("")
	c.setCredentials(nil)
	return nil
}

// SendHeartBeat sends a heartbeat message to the device.
func (c *Client) SendHeartBeat(ctx context.Context) error {
	_, err := c.conn.command(ctx, newCommand(cmdHeartBeat))
	return err
}

// Reboot reboots the HEOS device. The device does not respond to this
// command; when auto reconnect or failover is enabled the client will
// re-establish the connection once the device is back.
func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.conn.command(ctx, newCommand(cmdReboot))
	return err
}

// GetSystemInfo retrieves information about all devices in the HEOS system.
func (c *Client) GetSystemInfo(ctx context.Context) (*System, error) {
	resp, err := c.conn.command(ctx, newCommand(cmdGetPlayers))
	if err != nil {
		return nil, err
	}
	list, err := decodePayloadList(resp.payload)
	if err != nil {
		return nil, err
	}
	hosts := make([]Host, 0, len(list))
	for _, data := range list {
		hosts = append(hosts, hostFromData(data))
	}
	return newSystem(c.SignedInUsername(), hostOnly(c.conn.currentHost()), hosts), nil
}

// hostOnly strips the port from a "host:port" address for comparison against
// device-reported IP addresses.
func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func onOff(value bool) string {
	if value {
		return valueOn
	}
	return valueOff
}
