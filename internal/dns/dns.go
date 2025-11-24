package dns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager manages dnsmasq configuration for DNS-level blocking
type Manager struct {
	configPath string
}

// New creates a new DNS Manager
func New(configPath string) *Manager {
	return &Manager{
		configPath: configPath,
	}
}

// ApplyRules generates a dnsmasq configuration file that blocks the given domains
// This includes wildcard blocking for all subdomains
func (m *Manager) ApplyRules(domains []string) error {
	// Ensure the directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# focusd - DNS blocking configuration\n")
	sb.WriteString("# Auto-generated - do not edit manually\n\n")

	for _, domain := range domains {
		// Block the base domain
		sb.WriteString(fmt.Sprintf("address=/%s/0.0.0.0\n", domain))

		// Block all subdomains with wildcard
		// Note: dnsmasq treats /domain.com/ as matching domain.com and all subdomains
		// But we'll be explicit for clarity
		if !strings.HasPrefix(domain, "www.") {
			sb.WriteString(fmt.Sprintf("address=/www.%s/0.0.0.0\n", domain))
		}
	}

	// Write the configuration file
	if err := os.WriteFile(m.configPath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("writing dnsmasq config: %w", err)
	}

	return nil
}

// RemoveRules removes the dnsmasq configuration file
func (m *Manager) RemoveRules() error {
	if err := os.Remove(m.configPath); err != nil {
		// If the file doesn't exist, that's OK
		if !os.IsNotExist(err) {
			return fmt.Errorf("removing dnsmasq config: %w", err)
		}
	}
	return nil
}

// UpdateRules updates the DNS blocking rules with new domains
func (m *Manager) UpdateRules(domains []string) error {
	// For dnsmasq, we just regenerate the entire config
	return m.ApplyRules(domains)
}

// IsConfigured returns true if the dnsmasq config file exists
func (m *Manager) IsConfigured() bool {
	_, err := os.Stat(m.configPath)
	return err == nil
}
