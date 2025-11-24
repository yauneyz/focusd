package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"focusd/internal/config"
	"focusd/internal/daemon"
	"focusd/internal/state"
	"focusd/internal/usbkey"
)

var (
	configPath string
	cfg        *config.Config
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "focusd",
	Short: "focusd - A distraction blocker with USB key authentication",
	Long: `focusd blocks distracting websites using both DNS and nftables.
Enabling or disabling the blocker requires a USB key for authentication.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for commands that don't need it
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}

		// Load configuration
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return nil
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the focusd daemon",
	Long:  `Starts the focusd daemon which manages DNS and nftables blocking rules.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := daemon.New(cfg)
		return d.Run()
	},
}

var enableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable blocking (requires USB key)",
	Long:  `Enables the distraction blocker. Requires the USB key to be present.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Verify USB key
		verifier := usbkey.New(cfg.USBKeyPath, cfg.TokenHashPath)
		if err := verifier.Verify(); err != nil {
			return fmt.Errorf("USB key verification failed: %w", err)
		}

		// Update state
		st := state.New(state.DefaultStatePath)
		if err := st.SetEnabled(true); err != nil {
			return fmt.Errorf("updating state: %w", err)
		}

		fmt.Println("Blocker enabled successfully")
		fmt.Println("Run 'systemctl reload focusd' or send SIGHUP to apply changes")
		return nil
	},
}

var disableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable blocking (requires USB key)",
	Long:  `Disables the distraction blocker. Requires the USB key to be present.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Verify USB key
		verifier := usbkey.New(cfg.USBKeyPath, cfg.TokenHashPath)
		if err := verifier.Verify(); err != nil {
			return fmt.Errorf("USB key verification failed: %w", err)
		}

		// Update state
		st := state.New(state.DefaultStatePath)
		if err := st.SetEnabled(false); err != nil {
			return fmt.Errorf("updating state: %w", err)
		}

		fmt.Println("Blocker disabled successfully")
		fmt.Println("Run 'systemctl reload focusd' or send SIGHUP to apply changes")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current blocking status",
	Long:  `Displays whether the blocker is currently enabled or disabled.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		st := state.New(state.DefaultStatePath)
		status, err := st.String()
		if err != nil {
			return fmt.Errorf("reading status: %w", err)
		}

		fmt.Printf("focusd: %s\n", status)
		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/focusd/config.yaml", "path to config file")

	// Add subcommands
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(enableCmd)
	rootCmd.AddCommand(disableCmd)
	rootCmd.AddCommand(statusCmd)

	// Disable the completion command (optional)
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
