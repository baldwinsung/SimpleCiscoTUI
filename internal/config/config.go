// Package config loads the optional TOML device config for SimpleCiscoTUI.
//
// Save devices so you don't retype connection details. The smallest valid
// file is just a host:
//
//	[[devices]]
//	host = "172.16.0.1"
//
// With no password set, the app authenticates the same way `ssh 172.16.0.1`
// does -- your SSH agent and the keys in ~/.ssh -- and the username defaults
// to your local login. One saved device -> the app connects to it on launch.
//
// Search order (first match wins):
//  1. $SIMPLECISCOTUI_CONFIG
//  2. ./config.toml            (current directory)
//  3. ~/.config/simpleciscotui/config.toml
//
// The parsing here is pure (no I/O beyond reading the file) so it is unit-tested.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/cisco"
)

// ConfigEnv is the environment variable that overrides the config search path.
const ConfigEnv = "SIMPLECISCOTUI_CONFIG"

var allowedKeys = map[string]bool{
	"host":       true,
	"name":       true,
	"username":   true,
	"password":   true,
	"secret":     true,
	"port":       true,
	"key_file":   true,
	"legacy_ssh": true,
}

// ConfigError is raised for a malformed or unreadable config file.
type ConfigError struct{ msg string }

func (e *ConfigError) Error() string { return e.msg }

func newConfigError(format string, args ...any) *ConfigError {
	return &ConfigError{msg: fmt.Sprintf(format, args...)}
}

// DeviceConfig is one [[devices]] entry (after merging [defaults]).
type DeviceConfig struct {
	Host      string
	Name      string
	Username  string
	Password  string
	Secret    string
	Port      int
	KeyFile   string
	LegacySSH bool
}

// Label is the display string used in the device picker.
func (d DeviceConfig) Label() string {
	if d.Name != d.Host {
		return fmt.Sprintf("%s  (%s)", d.Name, d.Host)
	}
	return d.Host
}

// ToCredentials builds ssh credentials for this device, defaulting the
// username to the local login (like `ssh <host>`).
func (d DeviceConfig) ToCredentials() cisco.Credentials {
	username := d.Username
	if username == "" {
		username = currentUser()
	}
	return cisco.Credentials{
		Host:      d.Host,
		Username:  username,
		Password:  d.Password,
		Secret:    d.Secret,
		Port:      d.Port,
		KeyFile:   d.KeyFile,
		LegacySSH: d.LegacySSH,
	}
}

func currentUser() string {
	for _, key := range []string{"USER", "LOGNAME"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	if u, err := os.UserHomeDir(); err == nil && u != "" {
		return filepath.Base(u)
	}
	return ""
}

// Config is the parsed config file: its devices and the path it came from.
type Config struct {
	Devices []DeviceConfig
	Path    string
}

// ParseConfig parses TOML text into a list of DeviceConfig (pure).
func ParseConfig(text string) ([]DeviceConfig, error) {
	var data map[string]any
	if _, err := toml.Decode(text, &data); err != nil {
		return nil, newConfigError("invalid TOML: %s", err)
	}

	defaults := map[string]any{}
	if raw, ok := data["defaults"]; ok {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, newConfigError("`[defaults]` must be a table")
		}
		defaults = m
	}

	rawDevicesAny, ok := data["devices"]
	var rawDevices []map[string]any
	if ok {
		arr, ok := rawDevicesAny.([]map[string]any)
		if !ok {
			return nil, newConfigError("`devices` must be a list of [[devices]] tables")
		}
		rawDevices = arr
	}

	var devices []DeviceConfig
	for i, raw := range rawDevices {
		n := i + 1
		merged := map[string]any{}
		for k, v := range defaults {
			merged[k] = v
		}
		for k, v := range raw {
			merged[k] = v
		}

		hostVal, _ := merged["host"].(string)
		if hostVal == "" {
			return nil, newConfigError("device #%d is missing a `host`", n)
		}

		var unknown []string
		for k := range merged {
			if !allowedKeys[k] {
				unknown = append(unknown, k)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return nil, newConfigError("device #%d (%s) has unknown key(s): %s", n, hostVal, strings.Join(unknown, ", "))
		}

		dc := DeviceConfig{Host: hostVal, Port: 22}
		if v, ok := merged["name"].(string); ok {
			dc.Name = v
		}
		if v, ok := merged["username"].(string); ok {
			dc.Username = v
		}
		if v, ok := merged["password"].(string); ok {
			dc.Password = v
		}
		if v, ok := merged["secret"].(string); ok {
			dc.Secret = v
		}
		if v, ok := merged["port"]; ok {
			p, err := toInt(v)
			if err != nil {
				return nil, newConfigError("device #%d (%s) has invalid `port`: %v", n, hostVal, err)
			}
			dc.Port = p
		}
		if v, ok := merged["key_file"].(string); ok {
			dc.KeyFile = v
		}
		if v, ok := merged["legacy_ssh"].(bool); ok {
			dc.LegacySSH = v
		}
		if dc.Name == "" {
			dc.Name = dc.Host
		}
		devices = append(devices, dc)
	}
	return devices, nil
}

func toInt(v any) (int, error) {
	switch t := v.(type) {
	case int64:
		return int(t), nil
	case int:
		return t, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", v)
	}
}

func defaultPaths() []string {
	paths := []string{"config.toml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "simpleciscotui", "config.toml"))
	}
	return paths
}

// FindConfigPath returns the first config file that exists, or "".
func FindConfigPath() string {
	if env := os.Getenv(ConfigEnv); env != "" {
		candidate := expandHome(env)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
		return ""
	}
	for _, candidate := range defaultPaths() {
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
	}
	return ""
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// LoadConfig loads the config file (or returns an empty config if there is none).
func LoadConfig(path string) (Config, error) {
	if path == "" {
		path = FindConfigPath()
	}
	if path == "" {
		return Config{}, nil
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return Config{}, newConfigError("could not read %s: %s", path, err)
	}
	devices, err := ParseConfig(string(text))
	if err != nil {
		if ce, ok := err.(*ConfigError); ok {
			return Config{}, newConfigError("invalid TOML in %s: %s", path, ce.msg)
		}
		return Config{}, err
	}
	return Config{Devices: devices, Path: path}, nil
}
