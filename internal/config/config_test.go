package config

import (
	"strings"
	"testing"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/cisco"
)

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestMinimalHostOnly(t *testing.T) {
	devices, err := ParseConfig("[[devices]]\nhost = \"172.16.0.1\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.Host != "172.16.0.1" {
		t.Fatalf("host = %q", d.Host)
	}
	if d.Name != "172.16.0.1" { // name defaults to host
		t.Fatalf("name = %q", d.Name)
	}
	if d.Port != 22 {
		t.Fatalf("port = %d, want 22", d.Port)
	}
	if d.Label() != "172.16.0.1" {
		t.Fatalf("label = %q", d.Label())
	}
}

func TestDefaultsAreMergedAndOverridable(t *testing.T) {
	text := `
[defaults]
username = "admin"
port = 2222

[[devices]]
host = "172.16.0.1"
name = "SW1"

[[devices]]
host = "172.16.0.2"
port = 22
`
	devices, err := ParseConfig(text)
	if err != nil {
		t.Fatal(err)
	}
	if devices[0].Username != "admin" || devices[0].Port != 2222 {
		t.Fatalf("unexpected devices[0]: %+v", devices[0])
	}
	if devices[0].Name != "SW1" {
		t.Fatalf("name = %q", devices[0].Name)
	}
	if devices[0].Label() != "SW1  (172.16.0.1)" {
		t.Fatalf("label = %q", devices[0].Label())
	}
	// per-device value overrides the default
	if devices[1].Port != 22 {
		t.Fatalf("devices[1].Port = %d, want 22", devices[1].Port)
	}
}

func TestMissingHostRaises(t *testing.T) {
	_, err := ParseConfig("[[devices]]\nname = \"SW1\"\n")
	if err == nil || !strings.Contains(err.Error(), "missing a `host`") {
		t.Fatalf("got %v, want missing-host error", err)
	}
}

func TestUnknownKeyRaises(t *testing.T) {
	_, err := ParseConfig("[[devices]]\nhost = \"x\"\nuser = \"oops\"\n")
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("got %v, want unknown-key error", err)
	}
}

func TestNoPasswordUsesBatchmodeKeyAuth(t *testing.T) {
	devices, err := ParseConfig("[[devices]]\nhost = \"172.16.0.1\"\nusername = \"admin\"\n")
	if err != nil {
		t.Fatal(err)
	}
	creds := devices[0].ToCredentials()
	if creds.UsesPassword() {
		t.Fatal("expected UsesPassword() == false")
	}
	opts := creds.SSHOptions(cisco.DefaultConnTimeout)
	if !contains(opts, "BatchMode=yes") {
		t.Fatalf("opts = %v, want BatchMode=yes", opts)
	}
	if creds.Target() != "admin@172.16.0.1" {
		t.Fatalf("target = %q", creds.Target())
	}
}

func TestPasswordDisablesBatchmode(t *testing.T) {
	devices, err := ParseConfig("[[devices]]\nhost = \"x\"\nusername = \"a\"\npassword = \"p\"\n")
	if err != nil {
		t.Fatal(err)
	}
	opts := devices[0].ToCredentials().SSHOptions(cisco.DefaultConnTimeout)
	if contains(opts, "BatchMode=yes") {
		t.Fatalf("opts = %v, expected no BatchMode", opts)
	}
}

func TestKeyFileAndLegacySSHBecomeSSHFlags(t *testing.T) {
	text := "[[devices]]\nhost = \"x\"\nusername = \"u\"\nkey_file = \"~/k\"\nlegacy_ssh = true\n"
	devices, err := ParseConfig(text)
	if err != nil {
		t.Fatal(err)
	}
	opts := devices[0].ToCredentials().SSHOptions(cisco.DefaultConnTimeout)
	if !contains(opts, "-i") {
		t.Fatalf("opts = %v, want -i", opts)
	}
	joined := strings.Join(opts, " ")
	if !strings.Contains(joined, "ssh-rsa") {
		t.Fatalf("opts = %v, want ssh-rsa", opts)
	}
	if !strings.Contains(joined, "diffie-hellman-group14-sha1") {
		t.Fatalf("opts = %v, want diffie-hellman-group14-sha1", opts)
	}
}

func TestPortAddsPFlag(t *testing.T) {
	devices, err := ParseConfig("[[devices]]\nhost = \"x\"\nport = 2222\n")
	if err != nil {
		t.Fatal(err)
	}
	opts := devices[0].ToCredentials().SSHOptions(cisco.DefaultConnTimeout)
	if !contains(opts, "-p") || !contains(opts, "2222") {
		t.Fatalf("opts = %v, want -p 2222", opts)
	}
}

func TestUsernameDefaultsToLocalUser(t *testing.T) {
	t.Setenv("USER", "someuser")
	t.Setenv("LOGNAME", "someuser")
	devices, err := ParseConfig("[[devices]]\nhost = \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	creds := devices[0].ToCredentials()
	if creds.Username != "someuser" {
		t.Fatalf("username = %q, want someuser", creds.Username)
	}
	if creds.Target() != "someuser@x" {
		t.Fatalf("target = %q", creds.Target())
	}
}
