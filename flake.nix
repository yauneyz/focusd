{
  description = "focusd - A distraction blocker with USB key authentication";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "focusd";
          version = "0.1.0";

          src = ./.;

          vendorHash = "sha256-4MwBYiQBii4lE55qmGfsp/p9lqj1JlulGykd605+swg=";

          # Build only the main binary
          subPackages = [ "cmd/focusd" ];

          ldflags = [
            "-s"
            "-w"
          ];

          meta = with pkgs.lib; {
            description = "A distraction blocker with DNS and nftables integration";
            homepage = "https://github.yauneyz.com/focusd";
            license = licenses.mit;
            mainProgram = "focusd";
          };
        };

        # Development shell
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            nftables
          ];

          shellHook = ''
            echo "focusd development environment"
            echo "Run 'go build ./cmd/focusd' to build"
          '';
        };

        # NixOS module
        nixosModules.default = import ./nixos-module.nix;
      }
    ) // {
      # Overlay for use in NixOS configurations
      overlays.default = final: prev: {
        focusd = self.packages.${final.system}.default;
      };
    };
}
