package nft

import (
	"fmt"
	"net"

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
