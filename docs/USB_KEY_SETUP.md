# USB Key Setup Guide

The USB key acts as a physical "password" required to enable or disable the focusd blocker. This guide explains how to create and configure your USB key.

## Overview

The security mechanism works as follows:

1. You create a USB drive with a random key file
2. focusd stores the SHA256 hash of that key file (not the key itself)
3. To enable/disable blocking, you must plug in the USB drive
4. focusd verifies the USB key matches the stored hash before allowing state changes

This makes it much harder to impulsively disable the blocker - you need physical access to the specific USB key.

## Step 1: Choose a USB Drive

- Any USB drive will work (even a tiny 128MB one is fine)
- Ideally, use a dedicated USB drive and keep it somewhere inconvenient
- The drive doesn't need to be encrypted (the key file is just random data)
- You can use any filesystem (FAT32, ext4, etc.)

## Step 2: Create the Key File

### Option A: Auto-mounted USB (recommended)

Most Linux systems auto-mount USB drives to `/run/media/USERNAME/VOLUME_LABEL/`.

1. **Format the USB drive** (optional, but recommended for a clean start):
   ```bash
   # Find your USB device (e.g., /dev/sdb1)
   lsblk

   # Format as FAT32 with label FOCUSD
   sudo mkfs.vfat -n FOCUSD /dev/sdX1
   ```

2. **Mount the drive** (or let it auto-mount):
   ```bash
   # It should appear at /run/media/yourusername/FOCUSD
   ```

3. **Create the FOCUSD directory** (if using label other than FOCUSD):
   ```bash
   mkdir /run/media/yourusername/VOLUME_LABEL/FOCUSD
   ```

4. **Generate a random key file**:
   ```bash
   dd if=/dev/urandom of=/run/media/yourusername/FOCUSD/focusd.key bs=32 count=1
   ```

   This creates a 32-byte random file. The size doesn't matter much - even a few bytes would work.

5. **Generate the hash file for NixOS**:
   ```bash
   sha256sum /run/media/yourusername/FOCUSD/focusd.key > ~/token.sha256
   ```

### Option B: Manual Mount Path

If you prefer a specific mount point:

1. **Mount your USB drive**:
   ```bash
   sudo mount /dev/sdX1 /mnt/usb
   ```

2. **Create the key**:
   ```bash
   mkdir -p /mnt/usb/FOCUSD
   dd if=/dev/urandom of=/mnt/usb/FOCUSD/focusd.key bs=32 count=1
   ```

3. **Generate the hash**:
   ```bash
   sha256sum /mnt/usb/FOCUSD/focusd.key > ~/token.sha256
   ```

4. **Update your focusd config** to use the correct path:
   ```yaml
   usbKeyPath: "/mnt/usb/FOCUSD/focusd.key"
   ```

## Step 3: Add Hash to NixOS Configuration

The hash file should look like this:
```
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  focusd.key
```

Add this file to your NixOS configuration:

### Option A: Store hash file in your config repo

```nix
# In your configuration.nix or flake
services.focusd = {
  enable = true;
  tokenHashFile = ./token.sha256;  # Path relative to your config
  # ... other options
};
```

### Option B: Manually copy to /etc

```bash
sudo mkdir -p /etc/focusd
sudo cp ~/token.sha256 /etc/focusd/token.sha256
```

Then in your config:
```nix
services.focusd = {
  enable = true;
  tokenHashFile = /etc/focusd/token.sha256;
  # ... other options
};
```

## Step 4: Verify Setup

1. **Rebuild NixOS**:
   ```bash
   sudo nixos-rebuild switch
   ```

2. **Plug in your USB key**

3. **Test verification**:
   ```bash
   # Should work with USB plugged in
   sudo focusd enable

   # Should fail with USB unplugged
   sudo focusd disable  # Unplug USB first
   ```

## Security Best Practices

### Store the USB Key Inconveniently

The whole point is to make it harder to disable the blocker impulsively:

- Keep it in a drawer in another room
- Give it to a trusted friend or family member
- Put it in a time-locked safe
- Store it at work if blocking is for home distractions

### Create a Backup

You should create a backup USB key in case you lose the original:

1. Copy the key file to a second USB drive:
   ```bash
   cp /run/media/yourusername/FOCUSD/focusd.key /path/to/backup/
   ```

2. Store the backup somewhere very safe (not easily accessible)

3. The same hash file works for any identical copy of the key

### Don't Store the Key File on Your Computer

The key file should ONLY exist on the USB drive(s). Delete any copies:

```bash
# After generating the hash, delete local copies
rm ~/focusd.key  # if you accidentally copied it
```

The hash file is safe to store (it's a one-way hash), but the actual key file defeats the purpose if it's on your computer.

## Troubleshooting

### Error: "USB key not found"

Check that your USB is mounted at the expected path:
```bash
# Default path pattern
ls /run/media/*/FOCUSD/focusd.key

# See what's actually mounted
lsblk
mount | grep media
```

Adjust your config's `usbKeyPath` if needed:
```yaml
usbKeyPath: "/actual/mount/path/*/FOCUSD/focusd.key"
```

### Error: "USB key does not match expected token"

The hash doesn't match. Either:
- You're using the wrong USB key
- The key file was modified or corrupted
- The hash file doesn't match this key

Regenerate the hash:
```bash
sha256sum /run/media/yourusername/FOCUSD/focusd.key
```

Compare with the hash in `/etc/focusd/token.sha256`.

### Need to Create a New Key?

If you lost your USB key:

1. Create a new key following Step 2
2. Generate a new hash file
3. Update your NixOS configuration with the new hash
4. Run `sudo nixos-rebuild switch`

## Advanced: Using Multiple USB Keys

You can create multiple USB keys that all work:

1. **Option A: Copy the same key file to multiple USBs**
   - All copies have identical content
   - Same hash works for all of them

2. **Option B: Use multiple different keys** (requires code modification)
   - Would need to modify focusd to accept multiple hashes
   - Not currently supported, but could be added

## FAQ

**Q: Can someone just copy my USB key?**
A: Yes, physically copying the file would work. This is for self-control, not security against a determined attacker.

**Q: What if I lose my USB key?**
A: You'll need root access to modify the state file manually, or rebuild NixOS with a new key hash.

**Q: Can I use a file on my phone instead of USB?**
A: If you can mount your phone's storage at a predictable path, yes! Adjust `usbKeyPath` accordingly.

**Q: Does the USB need to stay plugged in?**
A: No, only during the `enable`/`disable` commands. The daemon doesn't check the USB continuously.

**Q: Can I use a hardware security key (YubiKey)?**
A: Not with the current implementation, but it could be modified to support challenge-response authentication.

## Support

If you encounter issues, check:
1. USB is mounted and path is correct
2. Key file exists and is readable
3. Hash file matches the key file
4. focusd service has permission to read the hash file

For more help, open an issue on the GitHub repository.
