package cisco

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

const (
	// DefaultConnTimeout is how long Connect waits for the ssh login prompt.
	DefaultConnTimeout = 25 * time.Second
	// DefaultCommandTimeout is how long a single device command may run.
	DefaultCommandTimeout = 60 * time.Second
)

// Session is an interactive IOS session over the system ssh client.
//
// ssh is spawned once under a pseudo-terminal (via creack/pty) and kept
// open; commands are sent one at a time and read back up to the device
// prompt. This is what lets old IOS -- which dribbles its interactive
// parser one line at a time -- work reliably, unlike piping a whole script
// at once.
type Session struct {
	Credentials    Credentials
	ConnTimeout    time.Duration
	CommandTimeout time.Duration

	mu     sync.Mutex
	ptmx   *os.File
	cmd    *exec.Cmd
	out    *outputBuffer
	online bool
}

// NewSession returns a Session with default timeouts.
func NewSession(creds Credentials) *Session {
	return &Session{
		Credentials:    creds,
		ConnTimeout:    DefaultConnTimeout,
		CommandTimeout: DefaultCommandTimeout,
	}
}

// Connected reports whether the session has an active ssh child.
func (s *Session) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.online
}

func (s *Session) sshArgv() []string {
	c := s.Credentials
	argv := append([]string{"ssh"}, c.SSHOptions(s.ConnTimeout)...)
	argv = append(argv, c.Target())
	if c.Password != "" {
		if _, err := exec.LookPath("sshpass"); err == nil {
			argv = append([]string{"sshpass", "-p", c.Password}, argv...)
		}
	}
	return argv
}

// -- lifecycle ---------------------------------------------------------- //

// Connect spawns the ssh child, syncs on the device prompt, enters
// privileged mode (if a secret is set) and disables paging.
func (s *Session) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := exec.LookPath("ssh"); err != nil {
		return errors.New("the `ssh` client was not found on PATH")
	}
	if s.Credentials.KeyFile != "" {
		if _, err := os.Stat(expandHome(s.Credentials.KeyFile)); err != nil {
			return fmt.Errorf("SSH key not found: %s", s.Credentials.KeyFile)
		}
	}

	argv := s.sshArgv()
	cmd := exec.Command(argv[0], argv[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	s.ptmx = ptmx
	s.cmd = cmd
	s.out = newOutputBuffer(ptmx)

	before, after, _, err := s.out.expect([]*regexp.Regexp{PromptRe}, s.ConnTimeout)
	if err != nil {
		detail := strings.TrimSpace(cleanOutput(before))
		msg := lastNonEmptyLine(detail)
		if msg == "" {
			msg = "ssh connection failed"
		}
		s.closeLocked()
		return errors.New(msg)
	}

	// Privileged mode + disable paging.
	if s.Credentials.Secret != "" && strings.HasSuffix(strings.TrimRight(after, " "), ">") {
		s.sendline("enable")
		_, after2, idx2, err := s.out.expect([]*regexp.Regexp{PasswordPromptRe, PromptRe}, s.ConnTimeout)
		if err != nil {
			s.closeLocked()
			return err
		}
		if idx2 == 0 || strings.Contains(strings.ToLower(after2), "assword") {
			s.sendline(s.Credentials.Secret)
			if _, _, _, err := s.out.expect([]*regexp.Regexp{PromptRe}, s.ConnTimeout); err != nil {
				s.closeLocked()
				return err
			}
		}
	}
	s.sendline("terminal length 0")
	if _, _, _, err := s.out.expect([]*regexp.Regexp{PromptRe}, s.ConnTimeout); err != nil {
		s.closeLocked()
		return err
	}
	s.online = true
	return nil
}

// Disconnect logs out gracefully (best effort) and tears down the child.
func (s *Session) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx != nil {
		func() {
			defer func() { recover() }() // best-effort graceful logout
			s.sendline("exit")
			s.out.expectEOF(5 * time.Second)
		}()
		s.closeLocked()
	}
	s.online = false
}

func (s *Session) closeLocked() {
	if s.ptmx != nil {
		s.ptmx.Close()
		s.ptmx = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
		s.cmd = nil
	}
}

func (s *Session) sendline(line string) {
	s.ptmx.Write([]byte(line + "\n"))
}

// -- read ----------------------------------------------------------------//

// ListInterfaces runs `show ip interface brief` and parses the result.
func (s *Session) ListInterfaces() ([]Interface, error) {
	out, err := s.run("show ip interface brief")
	if err != nil {
		return nil, err
	}
	return ParseInterfaceBrief(out), nil
}

// InterfaceAcls runs `show running-config interface <iface>` and parses ACL bindings.
func (s *Session) InterfaceAcls(iface string) (InterfaceAcls, error) {
	out, err := s.run("show running-config interface " + iface)
	if err != nil {
		return InterfaceAcls{}, err
	}
	return ParseInterfaceAcls(out), nil
}

// -- write -----------------------------------------------------------------//

// ApplyAcl binds acl to iface in the given direction.
func (s *Session) ApplyAcl(iface, acl, direction string) (string, error) {
	built, err := BuildApplyCommands(iface, acl, direction)
	if err != nil {
		return "", err
	}
	cmds := append([]string{"configure terminal"}, built...)
	cmds = append(cmds, "end")
	return s.runMany(cmds)
}

// RemoveAcl unbinds acl from iface in the given direction.
func (s *Session) RemoveAcl(iface, acl, direction string) (string, error) {
	built, err := BuildRemoveCommands(iface, acl, direction)
	if err != nil {
		return "", err
	}
	cmds := append([]string{"configure terminal"}, built...)
	cmds = append(cmds, "end")
	return s.runMany(cmds)
}

// SaveConfig runs `write memory` -- copy running-config to startup-config.
func (s *Session) SaveConfig() (string, error) {
	return s.run("write memory")
}

// -- internals ---------------------------------------------------------- //

func (s *Session) run(command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return "", errors.New("not connected")
	}
	s.sendline(command)
	before, _, _, err := s.out.expect([]*regexp.Regexp{PromptRe}, s.CommandTimeout)
	if err != nil {
		return "", fmt.Errorf("timed out waiting for prompt after %q", command)
	}
	out := cleanOutput(before)
	// Drop the first line (the echoed command itself).
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		return out[i+1:], nil
	}
	return "", nil
}

// runMany sends each command in turn (each call independently locks the
// session, mirroring the Python port's lack of a cross-command lock).
func (s *Session) runMany(commands []string) (string, error) {
	parts := make([]string, 0, len(commands))
	for _, cmd := range commands {
		out, err := s.run(cmd)
		if err != nil {
			return "", err
		}
		parts = append(parts, out)
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
