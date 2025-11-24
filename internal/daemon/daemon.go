package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"focusd/internal/config"
	"focusd/internal/dns"
	"focusd/internal/nft"
	"focusd/internal/proxy"
	"focusd/internal/resolver"
	"focusd/internal/state"
)

// Daemon is the main focusd daemon
type Daemon struct {
	cfg      *config.Config
	state    *state.State
	resolver *resolver.Resolver
	nftMgr   *nft.Manager
	dnsMgr   *dns.Manager
	proxy    *proxy.TransparentProxy
}

// New creates a new Daemon instance
func New(cfg *config.Config) *Daemon {
	return &Daemon{
		cfg:      cfg,
		state:    state.New(state.DefaultStatePath),
		resolver: resolver.New(),
		nftMgr:   nft.New(),
		dnsMgr:   dns.New(cfg.DnsmasqConfigPath),
	}
}

// Run starts the daemon and runs until interrupted
func (d *Daemon) Run() error {
	log.Println("focusd daemon starting...")

	// Check initial state
	enabled, err := d.state.IsEnabled()
	if err != nil {
		return fmt.Errorf("checking state: %w", err)
	}

	if enabled {
		log.Println("Blocking is enabled, applying rules...")
		if err := d.applyRules(); err != nil {
			return fmt.Errorf("applying initial rules: %w", err)
		}
	} else {
		log.Println("Blocking is disabled, ensuring rules are removed...")
		if err := d.removeRules(); err != nil {
			return fmt.Errorf("removing rules: %w", err)
		}
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Set up ticker for periodic IP refresh
	refreshInterval := time.Duration(d.cfg.RefreshIntervalMinutes) * time.Minute
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	log.Printf("Daemon running. Will refresh IPs every %v", refreshInterval)

	// Main loop
	for {
		select {
		case sig := <-sigChan:
			if sig == syscall.SIGHUP {
				// SIGHUP triggers a reload
				log.Println("Received SIGHUP, reloading...")
				if err := d.reload(); err != nil {
					log.Printf("Error reloading: %v", err)
				}
			} else {
				// SIGINT or SIGTERM triggers shutdown
				log.Printf("Received signal %v, shutting down...", sig)
				return nil
			}

		case <-ticker.C:
			// Periodic refresh
			enabled, err := d.state.IsEnabled()
			if err != nil {
				log.Printf("Error checking state: %v", err)
				continue
			}

			if enabled {
				log.Println("Refreshing blocked IPs...")
				if err := d.updateRules(); err != nil {
					log.Printf("Error updating rules: %v", err)
				}
			}
		}
	}
}

// applyRules applies DNS blocking, IP blocking, and transparent proxy
func (d *Daemon) applyRules() error {
	// Load blocklist (either from config or external file)
	domains, err := d.cfg.LoadBlocklist()
	if err != nil {
		return fmt.Errorf("loading blocklist: %w", err)
	}
	log.Printf("Loaded %d domains from blocklist", len(domains))

	// Apply DNS rules (first line of defense)
	if err := d.dnsMgr.ApplyRules(domains); err != nil {
		return fmt.Errorf("applying DNS rules: %w", err)
	}
	log.Printf("DNS rules applied for %d domains", len(domains))

	// Resolve domains to IPs and apply IP blocking
	// (This is optional - DNS + transparent proxy are the main defenses)
	ips, err := d.resolver.Resolve(domains)
	if err != nil {
		log.Printf("Warning: error resolving domains: %v", err)
	} else {
		log.Printf("Resolved %d IP addresses", len(ips))

		// Apply nftables IP blocking rules
		if err := d.nftMgr.ApplyRules(ips); err != nil {
			log.Printf("Warning: error applying nftables IP rules: %v", err)
		} else {
			log.Println("nftables IP blocking rules applied")
		}
	}

	// Start transparent proxy (catches DNS-over-HTTPS bypass attempts)
	d.proxy = proxy.New(domains)
	if err := d.proxy.Start(); err != nil {
		return fmt.Errorf("starting transparent proxy: %w", err)
	}
	log.Println("Transparent proxy started")

	// Enable transparent proxy nftables rules (TPROXY)
	if err := d.nftMgr.EnableTransparentProxy(proxy.HTTPPort, proxy.HTTPSPort); err != nil {
		// Try to clean up proxy if nftables fails
		d.proxy.Stop()
		d.proxy = nil
		return fmt.Errorf("enabling transparent proxy rules: %w", err)
	}
	log.Println("Transparent proxy nftables rules enabled")

	return nil
}

// removeRules removes DNS blocking, IP blocking, and transparent proxy
func (d *Daemon) removeRules() error {
	// Stop transparent proxy
	if d.proxy != nil {
		log.Println("Stopping transparent proxy...")
		if err := d.proxy.Stop(); err != nil {
			log.Printf("Warning: error stopping proxy: %v", err)
		}
		d.proxy = nil
	}

	// Disable transparent proxy nftables rules
	if err := d.nftMgr.DisableTransparentProxy(); err != nil {
		log.Printf("Warning: error disabling transparent proxy rules: %v", err)
	}

	// Remove DNS rules
	if err := d.dnsMgr.RemoveRules(); err != nil {
		log.Printf("Warning: error removing DNS rules: %v", err)
	}

	// Remove nftables IP blocking rules
	if err := d.nftMgr.RemoveRules(); err != nil {
		log.Printf("Warning: error removing nftables rules: %v", err)
	}

	log.Println("All rules removed")
	return nil
}

// updateRules updates the nftables rules with fresh IP resolutions
func (d *Daemon) updateRules() error {
	// Load blocklist (either from config or external file)
	domains, err := d.cfg.LoadBlocklist()
	if err != nil {
		return fmt.Errorf("loading blocklist: %w", err)
	}

	// Resolve domains to IPs
	ips, err := d.resolver.Resolve(domains)
	if err != nil {
		return fmt.Errorf("resolving domains: %w", err)
	}

	// Update nftables rules
	if err := d.nftMgr.UpdateRules(ips); err != nil {
		return fmt.Errorf("updating nftables rules: %w", err)
	}

	log.Printf("Rules updated with %d IPs", len(ips))
	return nil
}

// reload reloads the daemon's state and applies or removes rules accordingly
func (d *Daemon) reload() error {
	enabled, err := d.state.IsEnabled()
	if err != nil {
		return fmt.Errorf("checking state: %w", err)
	}

	if enabled {
		log.Println("Reloading: blocking is enabled")
		return d.applyRules()
	} else {
		log.Println("Reloading: blocking is disabled")
		return d.removeRules()
	}
}
