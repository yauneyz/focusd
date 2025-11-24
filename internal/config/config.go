package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the focusd configuration
type Config struct {
	// BlockedDomains is the list of domains to block
	BlockedDomains []string `yaml:"blockedDomains"`

	// RefreshIntervalMinutes specifies how often to refresh IP addresses
	RefreshIntervalMinutes int `yaml:"refreshIntervalMinutes"`

	// USBKeyPath is a glob pattern for finding the USB key file
	USBKeyPath string `yaml:"usbKeyPath"`

	// TokenHashPath is the path to the expected token hash file
	TokenHashPath string `yaml:"tokenHashPath"`

	// DnsmasqConfigPath is where to write the dnsmasq configuration
	DnsmasqConfigPath string `yaml:"dnsmasqConfigPath"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		BlockedDomains:         []string{},
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

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	if len(c.BlockedDomains) == 0 {
		return fmt.Errorf("no blocked domains specified")
	}

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
