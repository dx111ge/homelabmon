package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard",
	Long:  `Creates the data directory, generates a node identity, and writes default configuration.`,
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	dir := dataDir()
	fmt.Printf("Setting up HomeMonitor in %s\n", dir)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Generate node ID
	idPath := filepath.Join(dir, "node-id")
	if _, err := os.Stat(idPath); os.IsNotExist(err) {
		nodeID := uuid.New().String()
		if err := os.WriteFile(idPath, []byte(nodeID), 0600); err != nil {
			return fmt.Errorf("write node id: %w", err)
		}
		fmt.Printf("Generated node ID: %s\n", nodeID)
	} else {
		existing, _ := os.ReadFile(idPath)
		fmt.Printf("Existing node ID: %s\n", string(existing))
	}

	// Write default config
	cfgPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		defaultCfg := `# HomeMonitor configuration
bind: ":9600"
collect-interval: 30
log-level: info
# ui: false
# llm: ""
# scan: false
# peers:
#   - "192.168.1.10:9600"
`
		if err := os.WriteFile(cfgPath, []byte(defaultCfg), 0600); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		fmt.Printf("Wrote default config to %s\n", cfgPath)
	} else {
		fmt.Printf("Config already exists at %s\n", cfgPath)
	}

	fmt.Println("\nSetup complete. Start with: homelabmon --ui")
	return nil
}
