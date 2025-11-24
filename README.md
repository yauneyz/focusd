# focusd

A distraction blocker for NixOS with USB key authentication. focusd helps you stay focused by blocking distracting websites using both DNS-level and IP-level (nftables) filtering.

## Features

- **Dual-layer blocking**: DNS blocking via dnsmasq + IP blocking via nftables
- **USB key authentication**: Enable/disable blocking only with a physical USB key
- **Persistent state**: On/off state survives reboots and system rebuilds
- **Automatic subdomain blocking**: Blocking `youtube.com` also blocks `www.youtube.com`, `m.youtube.com`, etc.
- **Periodic IP refresh**: Domain IPs are periodically resolved to catch changes
- **NixOS integration**: Declarative configuration with proper systemd service

## How It Works

1. **DNS Blocking**: Generates dnsmasq configuration to return `0.0.0.0` for blocked domains
2. **IP Blocking**: Resolves blocked domains to IPs and creates nftables rules to drop packets
3. **USB Key Security**: Requires a USB key with a specific cryptographic token to enable/disable blocking
4. **State Persistence**: Stores enabled/disabled state in `/var/lib/focusd/state` which survives reboots

## Installation on NixOS

### 1. Create Your USB Key

First, create your USB authentication key (see [docs/USB_KEY_SETUP.md](docs/USB_KEY_SETUP.md) for details):

```bash
# Format USB and create FOCUSD directory
mkdir /path/to/usb/FOCUSD

# Generate random key
dd if=/dev/urandom of=/path/to/usb/FOCUSD/focusd.key bs=32 count=1

# Generate hash file for NixOS
sha256sum /path/to/usb/FOCUSD/focusd.key > token.sha256
```

### 2. Add to Your NixOS Configuration

In your `flake.nix`, add focusd as an input:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    focusd.url = "github:yourusername/focusd";
  };

  outputs = { self, nixpkgs, focusd }: {
    nixosConfigurations.yourhostname = nixpkgs.lib.nixosSystem {
      modules = [
        focusd.nixosModules.default
        ./configuration.nix
      ];
    };
  };
}
```

### 3. Configure in `configuration.nix`

```nix
{ config, pkgs, ... }:

{
  services.focusd = {
    enable = true;

    # Use the config file
    configFile = ./focusd-config.yaml;

    # Or configure directly
    blockedDomains = [
      "youtube.com"
      "twitter.com"
      "reddit.com"
      "facebook.com"
      "instagram.com"
    ];

    tokenHashFile = ./token.sha256;  # The hash file you created

    refreshIntervalMinutes = 60;
  };
}
```

### 4. Rebuild Your System

```bash
sudo nixos-rebuild switch
```

## Usage

### Check Status

```bash
focusd status
```

### Enable Blocking (requires USB key)

```bash
# Plug in your USB key first!
sudo focusd enable
sudo systemctl reload focusd
```

### Disable Blocking (requires USB key)

```bash
# Plug in your USB key first!
sudo focusd disable
sudo systemctl reload focusd
```

### Run Daemon Manually (for testing)

```bash
sudo focusd daemon --config /etc/focusd/config.yaml
```

## Configuration

See `config.example.yaml` for a full example configuration:

```yaml
blockedDomains:
  - youtube.com
  - twitter.com
  - reddit.com

refreshIntervalMinutes: 60

usbKeyPath: "/run/media/*/FOCUSD/focusd.key"
tokenHashPath: "/etc/focusd/token.sha256"
dnsmasqConfigPath: "/run/focusd/dnsmasq.conf"
```

## Development

### Build from Source

```bash
# Enter development shell
nix develop

# Build
go build ./cmd/focusd

# Run tests
go test ./...
```

### Build with Nix

```bash
nix build
```

## Architecture

```
┌─────────────────────────────────────────┐
│            focusd daemon                │
├─────────────────────────────────────────┤
│  - Reads config from YAML               │
│  - Checks state file (enabled/disabled) │
│  - Resolves domains → IPs               │
│  - Generates dnsmasq config             │
│  - Creates nftables rules               │
│  - Periodic refresh every N minutes     │
└─────────────────────────────────────────┘
           ↓                    ↓
    ┌──────────┐        ┌──────────────┐
    │ dnsmasq  │        │  nftables    │
    │          │        │              │
    │ DNS-level│        │ IP-level     │
    │ blocking │        │ blocking     │
    └──────────┘        └──────────────┘
```

## Security Considerations

- The USB key acts as a physical "password" to prevent impulsive disabling
- Root access can bypass the blocker (this is intentional - it's for self-control, not true security)
- State file is stored in `/var/lib/focusd/` which persists across reboots
- The token hash (not the token itself) is stored in `/etc/focusd/`

## Troubleshooting

### Blocker doesn't work after reboot

Check that the daemon is running:
```bash
systemctl status focusd
```

### USB key not recognized

Check the glob pattern in your config matches your USB mount point:
```bash
ls /run/media/*/FOCUSD/focusd.key
```

### DNS still resolves blocked domains

Ensure dnsmasq is configured correctly:
```bash
systemctl status dnsmasq
cat /run/focusd/dnsmasq.conf
```

### nftables rules not applied

Check nftables:
```bash
sudo nft list ruleset | grep focusd
```

## License

MIT License - See LICENSE file for details

## Contributing

Contributions welcome! Please open an issue or pull request.
