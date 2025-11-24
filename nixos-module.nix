{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.focusd;
in
{
  options.services.focusd = {
    enable = mkEnableOption "focusd distraction blocker";

    package = mkOption {
      type = types.package;
      default = pkgs.focusd;
      defaultText = literalExpression "pkgs.focusd";
      description = "The focusd package to use";
    };

    configFile = mkOption {
      type = types.path;
      description = "Path to the focusd configuration file";
      example = literalExpression "./focusd-config.yaml";
    };

    tokenHashFile = mkOption {
      type = types.path;
      description = "Path to the USB key token hash file";
      example = literalExpression "./token.sha256";
    };

    blockedDomains = mkOption {
      type = types.listOf types.str;
      default = [];
      description = "List of domains to block (used if configFile is not provided)";
      example = [ "youtube.com" "twitter.com" "reddit.com" ];
    };

    refreshIntervalMinutes = mkOption {
      type = types.int;
      default = 60;
      description = "How often to refresh IP addresses (in minutes)";
    };

    usbKeyPath = mkOption {
      type = types.str;
      default = "/run/media/*/FOCUSD/focusd.key";
      description = "Glob pattern for finding the USB key file";
    };
  };

  config = mkIf cfg.enable {
    # Ensure nftables is enabled
    networking.nftables.enable = true;

    # Install the package
    environment.systemPackages = [ cfg.package ];

    # Create config directory and files
    environment.etc = {
      "focusd/config.yaml" = mkIf (cfg.configFile != null) {
        source = cfg.configFile;
      };

      "focusd/token.sha256" = {
        source = cfg.tokenHashFile;
      };
    } // (optionalAttrs (cfg.configFile == null && cfg.blockedDomains != []) {
      "focusd/config.yaml" = {
        text = ''
          blockedDomains:
          ${concatMapStringsSep "\n" (d: "  - ${d}") cfg.blockedDomains}
          refreshIntervalMinutes: ${toString cfg.refreshIntervalMinutes}
          usbKeyPath: "${cfg.usbKeyPath}"
          tokenHashPath: "/etc/focusd/token.sha256"
          dnsmasqConfigPath: "/run/focusd/dnsmasq.conf"
        '';
      };
    });

    # Create state directory
    systemd.tmpfiles.rules = [
      "d /var/lib/focusd 0750 root root -"
      "d /run/focusd 0755 root root -"
    ];

    # Configure dnsmasq to use our config
    services.dnsmasq = {
      enable = true;
      settings = {
        conf-file = [ "/run/focusd/dnsmasq.conf" ];
      };
    };

    # Create the systemd service
    systemd.services.focusd = {
      description = "focusd - Distraction blocker daemon";
      after = [ "network-online.target" "nftables.service" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        ExecStart = "${cfg.package}/bin/focusd daemon --config /etc/focusd/config.yaml";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        Restart = "on-failure";
        RestartSec = "10s";

        # Security hardening
        User = "root";  # Required for nftables and binding to privileged ports
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [
          "/var/lib/focusd"
          "/run/focusd"
        ];
        PrivateTmp = true;
      };
    };

    # Reload dnsmasq when focusd config changes
    systemd.services.dnsmasq = {
      partOf = [ "focusd.service" ];
      after = [ "focusd.service" ];
    };
  };
}
