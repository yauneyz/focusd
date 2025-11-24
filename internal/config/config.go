package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the focusd configuration
type Config struct {
	// BlockedDomains is the list of domains to block (optional if BlocklistPath is set)
	BlockedDomains []string `yaml:"blockedDomains,omitempty"`

	// BlocklistPath is the path to a separate blocklist file
	// Default: ~/.config/focusd/blocklist.yml
	BlocklistPath string `yaml:"blocklistPath,omitempty"`

	// RefreshIntervalMinutes specifies how often to refresh IP addresses
	RefreshIntervalMinutes int `yaml:"refreshIntervalMinutes"`

	// USBKeyPath is a glob pattern for finding the USB key file
	USBKeyPath string `yaml:"usbKeyPath"`

	// TokenHashPath is the path to the expected token hash file
	TokenHashPath string `yaml:"tokenHashPath"`

	// DnsmasqConfigPath is where to write the dnsmasq configuration
	DnsmasqConfigPath string `yaml:"dnsmasqConfigPath"`
}

// Blocklist represents the structure of the blocklist file
type Blocklist struct {
	Domains []string `yaml:"domains"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		BlockedDomains:         []string{},
		BlocklistPath:          "~/.config/focusd/blocklist.yml",
		RefreshIntervalMinutes: 60,
		USBKeyPath:             "/run/media/*/FOCUSD/focusd.key",
		TokenHashPath:          "/etc/focusd/token.sha256",
		DnsmasqConfigPath:      "/run/focusd/dnsmasq.conf",
	}
}

// Load reads and parses a YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// If BlocklistPath wasn't set in config, use default
	if cfg.BlocklistPath == "" {
		cfg.BlocklistPath = "~/.config/focusd/blocklist.yml"
	}

	// Expand home directory in BlocklistPath
	cfg.BlocklistPath = expandPath(cfg.BlocklistPath)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	// Note: We don't validate BlockedDomains or BlocklistPath here
	// They will be validated at runtime when LoadBlocklist() is called

	if c.RefreshIntervalMinutes < 1 {
		return fmt.Errorf("refresh interval must be at least 1 minute")
	}

	if c.USBKeyPath == "" {
		return fmt.Errorf("USB key path cannot be empty")
	}

	if c.TokenHashPath == "" {
		return fmt.Errorf("token hash path cannot be empty")
	}

	if c.DnsmasqConfigPath == "" {
		return fmt.Errorf("dnsmasq config path cannot be empty")
	}

	return nil
}

// LoadBlocklist loads domains from the blocklist file
func (c *Config) LoadBlocklist() ([]string, error) {
	// If BlockedDomains is set in config, use that
	if len(c.BlockedDomains) > 0 {
		return c.BlockedDomains, nil
	}

	// Otherwise load from blocklist file
	if c.BlocklistPath == "" {
		return nil, fmt.Errorf("no blocklist path configured")
	}

	data, err := os.ReadFile(c.BlocklistPath)
	if err != nil {
		return nil, fmt.Errorf("reading blocklist file %s: %w", c.BlocklistPath, err)
	}

	var blocklist Blocklist
	if err := yaml.Unmarshal(data, &blocklist); err != nil {
		return nil, fmt.Errorf("parsing blocklist file: %w", err)
	}

	if len(blocklist.Domains) == 0 {
		return nil, fmt.Errorf("blocklist file contains no domains")
	}

	return blocklist.Domains, nil
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		// Fallback to $HOME if we can't get current user
		home := os.Getenv("HOME")
		if home != "" {
			return filepath.Join(home, path[1:])
		}
		return path
	}

	return filepath.Join(usr.HomeDir, path[1:])
}
