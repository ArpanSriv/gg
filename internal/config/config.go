// Package config loads and saves gg's config file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config is the content of config.json.
type Config struct {
	PAT                 string `json:"pat,omitempty"`
	BasePath            string `json:"base_path,omitempty"`
	DefaultOrg          string `json:"default_org,omitempty"`
	Protocol            string `json:"protocol,omitempty"` // "ssh" or "https"
	WriteVSCodeSettings *bool  `json:"write_vscode_settings,omitempty"`
}

// Path returns the config file location: $GG_CONFIG, else
// $XDG_CONFIG_HOME/gg/config.json, else <user config dir>/gg/config.json
// (%AppData%\gg\config.json on Windows).
func Path() (string, error) {
	if p := os.Getenv("GG_CONFIG"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		var err error
		if dir, err = os.UserConfigDir(); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "gg", "config.json"), nil
}

// Load reads the config file. A missing file gives an empty config.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &c, nil
}

// Save writes the config file (mode 0600, directory 0700).
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// DefaultBasePath is used when base_path is not set.
func DefaultBasePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "github.com"
	}
	return filepath.Join(home, "src", "github.com")
}

// Base returns the directory that holds <owner>/<repo>.
func (c *Config) Base() string {
	if c.BasePath != "" {
		return c.BasePath
	}
	return DefaultBasePath()
}

// VSCode reports whether to write .vscode/settings.json (default true).
func (c *Config) VSCode() bool {
	return c.WriteVSCodeSettings == nil || *c.WriteVSCodeSettings
}

// GitProtocol returns "ssh" or "https". When the config does not set it,
// gg follows the gh CLI's git_protocol setting, else https.
func (c *Config) GitProtocol() string {
	if c.Protocol != "" {
		return c.Protocol
	}
	out, err := exec.Command("gh", "config", "get", "git_protocol", "-h", "github.com").Output()
	if err == nil && strings.TrimSpace(string(out)) == "ssh" {
		return "ssh"
	}
	return "https"
}

// CloneURL returns the URL gg uses for github.com/<owner>/<name>.
func (c *Config) CloneURL(owner, name string) string {
	if c.GitProtocol() == "ssh" {
		return fmt.Sprintf("git@github.com:%s/%s.git", owner, name)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
}
