// Package cisco provides Cisco IOS device access for SimpleCiscoTUI.
//
// Connections are driven through the system ssh binary, not a Go SSH
// library. That means the app inherits everything in ~/.ssh/config (keys,
// host aliases, and the legacy crypto old IOS needs) and works wherever
// plain `ssh <host>` already works.
//
// The pure parsing/command helpers (ParseInterfaceBrief, ParseInterfaceAcls,
// BuildApplyCommands, ...) carry no I/O and are unit-tested against captured
// `show` output. Everything that touches the wire lives in session.go.
package cisco

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Directions are the two ACL bind directions IOS understands.
var Directions = []string{"in", "out"}

// Interface is one row of `show ip interface brief`.
type Interface struct {
	Name     string
	IP       string
	Status   string
	Protocol string
}

// IsUp reports whether both status and protocol are "up".
func (i Interface) IsUp() bool {
	return strings.EqualFold(i.Status, "up") && strings.EqualFold(i.Protocol, "up")
}

// InterfaceAcls holds the ACLs bound to an interface, keyed by direction.
type InterfaceAcls struct {
	Inbound  string
	Outbound string
}

// Get returns the ACL bound in the given direction ("in" or "out").
func (a InterfaceAcls) Get(direction string) string {
	if direction == "in" {
		return a.Inbound
	}
	return a.Outbound
}

// Binding is a (direction, aclName) pair.
type Binding struct {
	Direction string
	ACL       string
}

// Bindings returns the (direction, acl) pairs that are actually set.
func (a InterfaceAcls) Bindings() []Binding {
	var out []Binding
	if a.Inbound != "" {
		out = append(out, Binding{"in", a.Inbound})
	}
	if a.Outbound != "" {
		out = append(out, Binding{"out", a.Outbound})
	}
	return out
}

// Credentials describes how to reach one device via the system ssh client.
//
// With no Password the app uses key/agent auth and never blocks on a prompt
// (BatchMode=yes) -- exactly like `ssh <host>`. KeyFile and LegacySSH map
// onto `ssh -i` / `ssh -o` flags, so a device works even without a matching
// ~/.ssh/config block.
type Credentials struct {
	Host      string
	Username  string
	Password  string // only usable if `sshpass` is installed; key auth preferred
	Secret    string // enable secret; sent after login when set
	Port      int
	KeyFile   string // explicit private key path
	LegacySSH bool   // add the crypto old IOS (e.g. 2960G) requires
}

// UsesPassword reports whether these credentials carry a password.
func (c Credentials) UsesPassword() bool {
	return c.Password != ""
}

// Target returns "user@host", or just "host" with no username.
func (c Credentials) Target() string {
	if c.Username != "" {
		return c.Username + "@" + c.Host
	}
	return c.Host
}

// SSHOptions returns the `ssh` flags (without the target host) for this device.
func (c Credentials) SSHOptions(connTimeout time.Duration) []string {
	secs := int(connTimeout / time.Second)
	opts := []string{"-o", fmt.Sprintf("ConnectTimeout=%d", secs)}
	if c.Password == "" {
		// Key / agent auth only -- never hang waiting for a password prompt
		// the TUI can't display.
		opts = append(opts, "-o", "BatchMode=yes")
	}
	if c.Port != 0 && c.Port != 22 {
		opts = append(opts, "-p", strconv.Itoa(c.Port))
	}
	if c.KeyFile != "" {
		opts = append(opts, "-o", "IdentitiesOnly=yes", "-i", expandHome(c.KeyFile))
	}
	if c.LegacySSH {
		opts = append(opts,
			"-o", "KexAlgorithms=+diffie-hellman-group14-sha1,diffie-hellman-group-exchange-sha1",
			"-o", "HostKeyAlgorithms=+ssh-rsa",
			"-o", "PubkeyAcceptedAlgorithms=+ssh-rsa",
			"-o", "Ciphers=+aes128-cbc,aes192-cbc,aes256-cbc,3des-cbc",
		)
	}
	return opts
}

func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// --------------------------------------------------------------------------
// Pure parsing / command helpers (no I/O)
// --------------------------------------------------------------------------

var briefHeaderRe = regexp.MustCompile(`(?i)^\s*Interface\s+IP-Address`)

// ParseInterfaceBrief parses `show ip interface brief` into Interface rows.
func ParseInterfaceBrief(text string) []Interface {
	var interfaces []Interface
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" || briefHeaderRe.MatchString(line) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 6 {
			// Skips blank lines, echoed commands, and the device prompt.
			continue
		}
		name, ip := parts[0], parts[1]
		status, protocol := parts[len(parts)-2], parts[len(parts)-1]
		if name == "" || !isAsciiLetter(rune(name[0])) {
			continue
		}
		interfaces = append(interfaces, Interface{Name: name, IP: ip, Status: status, Protocol: protocol})
	}
	return interfaces
}

func isAsciiLetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

var aclBindingRe = regexp.MustCompile(`(?i)ip access-group\s+(\S+)\s+(in|out)`)

// ParseInterfaceAcls parses `ip access-group <name> <in|out>` lines from
// interface config.
func ParseInterfaceAcls(text string) InterfaceAcls {
	var acls InterfaceAcls
	for _, m := range aclBindingRe.FindAllStringSubmatch(text, -1) {
		name, direction := m[1], strings.ToLower(m[2])
		if direction == "in" {
			acls.Inbound = name
		} else {
			acls.Outbound = name
		}
	}
	return acls
}

// NormalizeDirection lower-cases and validates a direction string.
func NormalizeDirection(direction string) (string, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "in" && direction != "out" {
		return "", fmt.Errorf("direction must be one of %v, got %q", Directions, direction)
	}
	return direction, nil
}

// BuildApplyCommands returns the config-mode commands to bind acl to interface.
func BuildApplyCommands(iface, acl, direction string) ([]string, error) {
	direction, err := NormalizeDirection(direction)
	if err != nil {
		return nil, err
	}
	return []string{
		"interface " + iface,
		fmt.Sprintf("ip access-group %s %s", acl, direction),
	}, nil
}

// BuildRemoveCommands returns the config-mode commands to unbind acl from interface.
func BuildRemoveCommands(iface, acl, direction string) ([]string, error) {
	direction, err := NormalizeDirection(direction)
	if err != nil {
		return nil, err
	}
	return []string{
		"interface " + iface,
		fmt.Sprintf("no ip access-group %s %s", acl, direction),
	}, nil
}

// PromptRe matches an IOS exec/config prompt at the end of the buffer, e.g.
// "c2960r1#", "c2960r1>", "c2960r1(config)#", "c2960r1(config-if)#".
var PromptRe = regexp.MustCompile(`[A-Za-z0-9._\-]+(?:\([^)]+\))?[>#] ?$`)

// PasswordPromptRe matches ssh/enable password prompts.
var PasswordPromptRe = regexp.MustCompile(`[Pp]assword:`)

// cleanOutput normalises raw pty output: strip CRs.
func cleanOutput(text string) string {
	return strings.ReplaceAll(text, "\r", "")
}
