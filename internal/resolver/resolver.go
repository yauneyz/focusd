package resolver

import (
	"fmt"
	"net"
	"strings"
)

// Resolver resolves domain names to IP addresses
type Resolver struct{}

// New creates a new Resolver
func New() *Resolver {
	return &Resolver{}
}

// Resolve resolves a list of domains to their IP addresses
// For each domain, it also resolves the www. subdomain variant
// Returns a deduplicated list of IP addresses (both IPv4 and IPv6)
func (r *Resolver) Resolve(domains []string) ([]net.IP, error) {
	ipSet := make(map[string]net.IP)

	for _, domain := range domains {
		// Resolve the base domain
		ips, err := r.resolveDomain(domain)
		if err != nil {
			// Log the error but continue with other domains
			fmt.Printf("Warning: failed to resolve %s: %v\n", domain, err)
			continue
		}
		for _, ip := range ips {
			ipSet[ip.String()] = ip
		}

		// Also resolve www. subdomain if not already included
		if !strings.HasPrefix(domain, "www.") {
			wwwDomain := "www." + domain
			ips, err := r.resolveDomain(wwwDomain)
			if err != nil {
				// It's OK if www subdomain doesn't exist
				continue
			}
			for _, ip := range ips {
				ipSet[ip.String()] = ip
			}
		}
	}

	// Convert map to slice
	result := make([]net.IP, 0, len(ipSet))
	for _, ip := range ipSet {
		result = append(result, ip)
	}

	return result, nil
}

// resolveDomain resolves a single domain to its IP addresses
func (r *Resolver) resolveDomain(domain string) ([]net.IP, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// GetDomainVariants returns all variants of a domain that should be blocked
// For example, "example.com" -> ["example.com", "www.example.com"]
func GetDomainVariants(domain string) []string {
	variants := []string{domain}

	// Add www. variant if not already present
	if !strings.HasPrefix(domain, "www.") {
		variants = append(variants, "www."+domain)
	}

	return variants
}
