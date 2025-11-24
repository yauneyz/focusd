package nft

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	tableName = "focusd"
	setName   = "blocked_ips"
	chainName = "output"
)

// Manager manages nftables rules for blocking IPs
type Manager struct {
	conn *nftables.Conn
}

// New creates a new nftables Manager
func New() *Manager {
	return &Manager{
		conn: &nftables.Conn{},
	}
}

// ApplyRules creates or updates nftables rules to block the given IP addresses
func (m *Manager) ApplyRules(ips []net.IP) error {
	// Create or get the table
	table := &nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   tableName,
	}
	m.conn.AddTable(table)

	// Create or get the set for blocked IPs
	set := &nftables.Set{
		Table:   table,
		Name:    setName,
		KeyType: nftables.TypeIPAddr,
	}
	if err := m.conn.AddSet(set, nil); err != nil {
		return fmt.Errorf("creating IP set: %w", err)
	}

	// Add IP addresses to the set
	elements := make([]nftables.SetElement, 0, len(ips))
	for _, ip := range ips {
		elements = append(elements, nftables.SetElement{
			Key: ip,
		})
	}
	if err := m.conn.SetAddElements(set, elements); err != nil {
		return fmt.Errorf("adding IP elements to set: %w", err)
	}

	// Create output chain if it doesn't exist
	policy := nftables.ChainPolicyAccept
	chain := &nftables.Chain{
		Name:     chainName,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policy,
	}
	m.conn.AddChain(chain)

	// Add rule to drop packets to blocked IPs
	// Rule: ip daddr @blocked_ips drop
	m.conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			// Load destination address into register 1
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       16, // Destination IP offset in IP header
				Len:          4,  // IPv4 address length (will handle IPv6 separately)
			},
			// Check if destination IP is in the blocked set
			&expr.Lookup{
				SourceRegister: 1,
				SetName:        setName,
			},
			// Drop the packet if it matches
			&expr.Verdict{
				Kind: expr.VerdictDrop,
			},
		},
	})

	// Flush all changes
	if err := m.conn.Flush(); err != nil {
		return fmt.Errorf("flushing nftables changes: %w", err)
	}

	return nil
}

// RemoveRules removes all focusd nftables rules
func (m *Manager) RemoveRules() error {
	// Get the table
	table := &nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   tableName,
	}

	// Delete the entire table (this removes all chains, sets, and rules)
	m.conn.DelTable(table)

	// Flush changes
	if err := m.conn.Flush(); err != nil {
		// If the table doesn't exist, that's OK
		if err != unix.ENOENT {
			return fmt.Errorf("removing nftables rules: %w", err)
		}
	}

	return nil
}

// UpdateRules updates the blocked IP list
// This clears the old set and replaces it with new IPs
func (m *Manager) UpdateRules(ips []net.IP) error {
	// For simplicity, we remove and re-apply rules
	// In production, you might want to do a more intelligent diff
	if err := m.RemoveRules(); err != nil {
		return err
	}
	return m.ApplyRules(ips)
}

// EnableTransparentProxy sets up nftables rules for transparent proxying
// This redirects HTTP and HTTPS traffic to the transparent proxy ports
func (m *Manager) EnableTransparentProxy(httpPort, httpsPort int) error {
	// Use nft command-line tool for TPROXY setup as it's complex
	// The nftables Go library doesn't have good TPROXY support

	rules := fmt.Sprintf(`
table inet focusd_proxy {
	chain prerouting {
		type filter hook prerouting priority mangle; policy accept;

		# Skip local traffic
		ip daddr 127.0.0.0/8 return
		ip6 daddr ::1/128 return

		# Skip private networks
		ip daddr 10.0.0.0/8 return
		ip daddr 172.16.0.0/12 return
		ip daddr 192.168.0.0/16 return

		# Intercept HTTP traffic
		tcp dport 80 tproxy ip to 127.0.0.1:%d mark set 1 accept
		tcp dport 80 tproxy ip6 to [::1]:%d mark set 1 accept

		# Intercept HTTPS traffic
		tcp dport 443 tproxy ip to 127.0.0.1:%d mark set 1 accept
		tcp dport 443 tproxy ip6 to [::1]:%d mark set 1 accept

		# Block QUIC (HTTP/3) to force TCP fallback
		udp dport 443 drop
	}

	chain output {
		type route hook output priority mangle; policy accept;

		# Skip proxy's own outbound connections (marked with fwmark 50)
		meta mark 50 return

		# Skip local traffic
		ip daddr 127.0.0.0/8 return
		ip6 daddr ::1/128 return

		# Skip private networks
		ip daddr 10.0.0.0/8 return
		ip daddr 172.16.0.0/12 return
		ip daddr 192.168.0.0/16 return

		# Intercept HTTP from local machine
		tcp dport 80 mark set 1 accept

		# Intercept HTTPS from local machine
		tcp dport 443 mark set 1 accept

		# Block QUIC
		udp dport 443 drop
	}

	chain output_nat {
		type nat hook output priority -100; policy accept;

		# Skip proxy's own outbound connections
		meta mark 50 return

		# Skip local traffic
		ip daddr 127.0.0.0/8 return
		ip6 daddr ::1/128 return

		# Skip private networks
		ip daddr 10.0.0.0/8 return
		ip daddr 172.16.0.0/12 return
		ip daddr 192.168.0.0/16 return

		# Redirect locally-generated HTTP to proxy
		tcp dport 80 redirect to :%d

		# Redirect locally-generated HTTPS to proxy
		tcp dport 443 redirect to :%d
	}
}
`, httpPort, httpPort, httpsPort, httpsPort, httpPort, httpsPort)

	// Apply rules using nft -f
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(rules)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("applying transparent proxy rules: %w (stderr: %s)", err, stderr.String())
	}

	// Set up routing for marked packets
	if err := setupRouting(); err != nil {
		return fmt.Errorf("setting up routing: %w", err)
	}

	return nil
}

// DisableTransparentProxy removes transparent proxy rules
func (m *Manager) DisableTransparentProxy() error {
	// Delete the proxy table
	cmd := exec.Command("nft", "delete", "table", "inet", "focusd_proxy")
	if err := cmd.Run(); err != nil {
		// If table doesn't exist, that's OK
		return nil
	}

	// Clean up routing
	cleanupRouting()

	return nil
}

// setupRouting configures routing policy for marked packets
func setupRouting() error {
	// IPv4: Add routing rule and route for marked packets
	commands := [][]string{
		{"ip", "rule", "add", "fwmark", "1", "lookup", "100"},
		{"ip", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", "100"},
		{"ip", "-6", "rule", "add", "fwmark", "1", "lookup", "100"},
		{"ip", "-6", "route", "add", "local", "::/0", "dev", "lo", "table", "100"},
	}

	for _, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		// Ignore errors - rules might already exist
		cmd.Run()
	}

	return nil
}

// cleanupRouting removes routing policy
func cleanupRouting() {
	commands := [][]string{
		{"ip", "rule", "del", "fwmark", "1", "lookup", "100"},
		{"ip", "route", "del", "local", "0.0.0.0/0", "dev", "lo", "table", "100"},
		{"ip", "-6", "rule", "del", "fwmark", "1", "lookup", "100"},
		{"ip", "-6", "route", "del", "local", "::/0", "dev", "lo", "table", "100"},
	}

	for _, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Run() // Ignore errors
	}
}
