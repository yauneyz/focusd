# Updating focusd in NixOS

This guide explains how to configure and update focusd in your NixOS configuration.

## Configuration

### 1. Flake Input

Your NixOS configuration at `~/nixos-config` should have focusd configured as a flake input:

```nix
focusd = {
  url = "git+file:///home/zac/development/tools/focusd";
  flake = false;
};
```

### 2. Service Configuration

In your host configuration (e.g., `hosts/laptop/default.nix`), enable focusd:

```nix
services.focusd = {
  enable = true;
  tokenHashFile = ../../secrets/focusd-token.sha256;
  blocklistFile = ../../secrets/blocklist.yml;
};
```

### 3. Blocklist File

Create a `blocklist.yml` file in your NixOS config (e.g., `~/nixos-config/secrets/blocklist.yml`):

```yaml
domains:
  - youtube.com
  - twitter.com
  - reddit.com
  # Add more domains as needed
```

**Important:** The blocklist file must be tracked in git for the flake to access it:

```bash
cd ~/nixos-config
git add secrets/blocklist.yml
```

## Updating focusd

When you make changes to the focusd source code, follow these steps to update your system:

### 1. Update the flake input

Navigate to your NixOS config and update the focusd input:

```bash
cd ~/nixos-config
nix flake lock --update-input focusd
```

This will update the flake.lock file to reference the latest commit in your local focusd repository.

### 2. Rebuild NixOS configuration

Rebuild and switch your NixOS configuration (replace `<hostname>` with `desktop`, `laptop`, or `vm`):

```bash
sudo nixos-rebuild switch --flake .#<hostname>
```

The focusd service will automatically restart as part of the rebuild.

### Quick Reference

One-liner to update and rebuild (replace `<hostname>`):

```bash
cd ~/nixos-config && nix flake lock --update-input focusd && sudo nixos-rebuild switch --flake .#<hostname>
```

## Managing the Blocklist

To update which domains are blocked, edit the blocklist file:

```bash
# Edit the blocklist
vim ~/nixos-config/secrets/blocklist.yml

# Commit the changes (required for flakes)
cd ~/nixos-config
git add secrets/blocklist.yml
git commit -m "Update focusd blocklist"

# Rebuild to apply changes
sudo nixos-rebuild switch --flake .#<hostname>
```

## Verifying the Update

Check the service status:

```bash
sudo systemctl status focusd
```

View logs:

```bash
sudo journalctl -u focusd -f
```
