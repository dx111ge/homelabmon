package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dx111ge/homelabmon/internal/mesh"
	"github.com/dx111ge/homelabmon/internal/store"
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
	setupCmd.Flags().Bool("systemd", false, "generate and install a systemd service unit (Linux only)")
	setupCmd.Flags().Bool("launchd", false, "generate and install a launchd plist (macOS only)")
	setupCmd.Flags().Bool("windows-service", false, "install as a Windows service")
	setupCmd.Flags().Bool("uninstall", false, "remove the installed service instead of installing")
	setupCmd.Flags().Bool("gen-ca", false, "generate a mesh CA (for mTLS between peers)")
	setupCmd.Flags().Bool("gen-token", false, "generate a one-time enrollment token for a new node")
}

func runSetup(cmd *cobra.Command, args []string) error {
	dir := dataDir()
	fmt.Printf("Setting up HomelabMon in %s\n", dir)

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
		defaultCfg := `# HomelabMon configuration
bind: ":9600"
collect-interval: 30
scan-interval: 300              # network scan interval in seconds (ARP + mDNS staggered)
log-level: info
# site: ""                    # site label for multi-site federation
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

	// CA generation
	if genCA, _ := cmd.Flags().GetBool("gen-ca"); genCA {
		return setupCA(dir)
	}

	// Generate enrollment token
	if genToken, _ := cmd.Flags().GetBool("gen-token"); genToken {
		return setupEnrollToken(dir)
	}

	uninstall, _ := cmd.Flags().GetBool("uninstall")

	// Service installers
	if systemd, _ := cmd.Flags().GetBool("systemd"); systemd {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("--systemd is only supported on Linux")
		}
		if uninstall {
			return uninstallSystemdService()
		}
		return installSystemdService()
	}

	if launchd, _ := cmd.Flags().GetBool("launchd"); launchd {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("--launchd is only supported on macOS")
		}
		if uninstall {
			return uninstallLaunchdService()
		}
		return installLaunchdService()
	}

	if winSvc, _ := cmd.Flags().GetBool("windows-service"); winSvc {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("--windows-service is only supported on Windows")
		}
		if uninstall {
			return uninstallWindowsService()
		}
		return installWindowsService()
	}

	fmt.Println("\nSetup complete. Start with: homelabmon --ui")
	fmt.Println("\nTo install as a service:")
	switch runtime.GOOS {
	case "linux":
		fmt.Println("  homelabmon setup --systemd")
	case "darwin":
		fmt.Println("  homelabmon setup --launchd")
	case "windows":
		fmt.Println("  homelabmon setup --windows-service")
	}
	return nil
}

// --- PKI / mTLS ---

func setupCA(dir string) error {
	pki := mesh.NewPKI(dir)
	if pki.CAExists() {
		fmt.Println("CA already exists. Delete ca.crt and ca.key to regenerate.")
		return nil
	}

	fmt.Println("Generating mesh CA ...")
	if err := pki.GenerateCA(); err != nil {
		return fmt.Errorf("generate CA: %w", err)
	}
	fmt.Printf("  CA cert: %s\n", filepath.Join(dir, "ca.crt"))
	fmt.Printf("  CA key:  %s\n", filepath.Join(dir, "ca.key"))

	// Also generate a node cert for this node
	idPath := filepath.Join(dir, "node-id")
	nodeID, _ := os.ReadFile(idPath)
	if len(nodeID) > 0 {
		fmt.Println("Generating node certificate ...")
		if err := pki.GenerateNodeCert(string(nodeID)); err != nil {
			return fmt.Errorf("generate node cert: %w", err)
		}
		fmt.Printf("  Node cert: %s\n", filepath.Join(dir, "node.crt"))
		fmt.Printf("  Node key:  %s\n", filepath.Join(dir, "node.key"))
	}

	fmt.Println("\nmTLS enabled. Peers need the CA cert to connect.")
	fmt.Println("To enroll a new node:")
	fmt.Println("  1. On this node:  homelabmon setup --gen-token")
	fmt.Println("  2. On new node:   homelabmon --enroll-url https://THIS_IP:9600 --enroll-token TOKEN")
	return nil
}

func setupEnrollToken(dir string) error {
	st, err := store.New(dir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	token := mesh.GenerateEnrollToken()
	if err := st.SetSetting(context.Background(), "enroll-token", token); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	fmt.Printf("Enrollment token (one-time use):\n\n  %s\n\n", token)
	fmt.Println("On the new node, run:")
	fmt.Printf("  homelabmon --enroll-url https://THIS_IP:9600 --enroll-token %s\n\n", token)
	fmt.Println("The token is invalidated after a single use.")
	return nil
}

// --- systemd (Linux) ---

func installSystemdService() error {
	binPath, err := exec.LookPath("homelabmon")
	if err != nil {
		binPath = "/usr/local/bin/homelabmon"
	}
	binPath, _ = filepath.Abs(binPath)

	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	unit := fmt.Sprintf(`[Unit]
Description=HomelabMon monitoring agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
ExecStart=%s --ui --scan
Restart=on-failure
RestartSec=5
AmbientCapabilities=CAP_NET_RAW
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, u.Username, u.Username, binPath)

	unitPath := "/etc/systemd/system/homelabmon.service"

	if os.Getuid() != 0 {
		tmpFile := filepath.Join(os.TempDir(), "homelabmon.service")
		if err := os.WriteFile(tmpFile, []byte(unit), 0644); err != nil {
			return fmt.Errorf("write temp unit: %w", err)
		}
		fmt.Printf("Installing systemd service (requires sudo) ...\n")
		cmds := [][]string{
			{"sudo", "cp", tmpFile, unitPath},
			{"sudo", "systemctl", "daemon-reload"},
			{"sudo", "systemctl", "enable", "homelabmon"},
			{"sudo", "systemctl", "start", "homelabmon"},
		}
		for _, c := range cmds {
			fmt.Printf("  %s\n", strings.Join(c, " "))
			out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %s %w", strings.Join(c, " "), string(out), err)
			}
		}
		os.Remove(tmpFile)
	} else {
		if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
			return fmt.Errorf("write unit file: %w", err)
		}
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "enable", "homelabmon").Run()
		exec.Command("systemctl", "start", "homelabmon").Run()
	}

	fmt.Printf("\nSystemd service installed and started:\n")
	fmt.Printf("  Unit file: %s\n", unitPath)
	fmt.Printf("  Binary:    %s\n", binPath)
	fmt.Printf("  User:      %s\n", u.Username)
	fmt.Printf("\nUseful commands:\n")
	fmt.Printf("  sudo systemctl status homelabmon    # check status\n")
	fmt.Printf("  sudo journalctl -u homelabmon -f    # follow logs\n")
	fmt.Printf("  sudo systemctl restart homelabmon   # restart\n")
	fmt.Printf("  sudo systemctl stop homelabmon      # stop\n")
	fmt.Printf("  homelabmon setup --systemd --uninstall   # remove service\n")
	return nil
}

func uninstallSystemdService() error {
	fmt.Println("Removing systemd service ...")
	prefix := "sudo"
	if os.Getuid() == 0 {
		prefix = ""
	}
	cmds := []string{
		prefix + " systemctl stop homelabmon",
		prefix + " systemctl disable homelabmon",
		prefix + " rm -f /etc/systemd/system/homelabmon.service",
		prefix + " systemctl daemon-reload",
	}
	for _, c := range cmds {
		c = strings.TrimSpace(c)
		fmt.Printf("  %s\n", c)
		parts := strings.Fields(c)
		exec.Command(parts[0], parts[1:]...).Run()
	}
	fmt.Println("Systemd service removed.")
	return nil
}

// --- launchd (macOS) ---

func installLaunchdService() error {
	binPath, err := exec.LookPath("homelabmon")
	if err != nil {
		binPath = "/usr/local/bin/homelabmon"
	}
	binPath, _ = filepath.Abs(binPath)

	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	logPath := filepath.Join(u.HomeDir, ".homelabmon", "homelabmon.log")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.homelabmon.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--ui</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, binPath, logPath, logPath)

	plistPath := filepath.Join(u.HomeDir, "Library", "LaunchAgents", "com.homelabmon.agent.plist")
	plistDir := filepath.Dir(plistPath)
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	fmt.Printf("Installing launchd service ...\n")
	out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %s %w", string(out), err)
	}

	fmt.Printf("\nLaunchd service installed and started:\n")
	fmt.Printf("  Plist:  %s\n", plistPath)
	fmt.Printf("  Binary: %s\n", binPath)
	fmt.Printf("  Log:    %s\n", logPath)
	fmt.Printf("\nUseful commands:\n")
	fmt.Printf("  launchctl list | grep homelabmon      # check status\n")
	fmt.Printf("  tail -f %s                            # follow logs\n", logPath)
	fmt.Printf("  launchctl stop com.homelabmon.agent   # stop\n")
	fmt.Printf("  launchctl start com.homelabmon.agent  # start\n")
	fmt.Printf("  homelabmon setup --launchd --uninstall     # remove service\n")
	return nil
}

func uninstallLaunchdService() error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}
	plistPath := filepath.Join(u.HomeDir, "Library", "LaunchAgents", "com.homelabmon.agent.plist")

	fmt.Println("Removing launchd service ...")
	exec.Command("launchctl", "unload", "-w", plistPath).Run()
	os.Remove(plistPath)
	fmt.Println("Launchd service removed.")
	return nil
}

// --- Windows Service ---

func installWindowsService() error {
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	binPath, _ = filepath.Abs(binPath)

	svcName := "HomelabMon"
	displayName := "HomelabMon Monitoring Agent"

	// Use sc.exe to create the service
	// binobj needs to be the full command including args
	binObj := fmt.Sprintf("\"%s\" --ui --scan", binPath)

	fmt.Printf("Installing Windows service ...\n")
	fmt.Printf("  Binary: %s\n", binPath)
	fmt.Printf("  Name:   %s\n", svcName)

	// Create the service
	out, err := exec.Command("sc.exe", "create", svcName,
		"binPath=", binObj,
		"DisplayName=", displayName,
		"start=", "auto",
	).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "5") {
			return fmt.Errorf("access denied -- run this command as Administrator (right-click terminal > Run as administrator)")
		}
		return fmt.Errorf("sc create: %s %w", string(out), err)
	}
	fmt.Printf("  %s\n", strings.TrimSpace(string(out)))

	// Set description
	exec.Command("sc.exe", "description", svcName,
		"Homelab monitoring agent with auto-discovery, mesh networking, and web dashboard.",
	).Run()

	// Set recovery: restart on failure after 10 seconds
	exec.Command("sc.exe", "failure", svcName,
		"reset=", "86400",
		"actions=", "restart/10000/restart/10000/restart/30000",
	).Run()

	// Start the service
	out, err = exec.Command("sc.exe", "start", svcName).CombinedOutput()
	if err != nil {
		fmt.Printf("  Note: could not start service automatically: %s\n", strings.TrimSpace(string(out)))
		fmt.Printf("  You may need to run this command as Administrator.\n")
	} else {
		fmt.Printf("  Service started.\n")
	}

	fmt.Printf("\nWindows service installed:\n")
	fmt.Printf("  Name:    %s\n", svcName)
	fmt.Printf("  Binary:  %s\n", binPath)
	fmt.Printf("  Startup: automatic\n")
	fmt.Printf("\nUseful commands:\n")
	fmt.Printf("  sc query %s              # check status\n", svcName)
	fmt.Printf("  sc stop %s               # stop\n", svcName)
	fmt.Printf("  sc start %s              # start\n", svcName)
	fmt.Printf("  homelabmon setup --windows-service --uninstall  # remove\n")
	fmt.Printf("\nNote: service management requires Administrator privileges.\n")
	return nil
}

func uninstallWindowsService() error {
	svcName := "HomelabMon"
	fmt.Println("Removing Windows service ...")

	// Stop first (ignore errors if not running)
	exec.Command("sc.exe", "stop", svcName).Run()

	out, err := exec.Command("sc.exe", "delete", svcName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc delete: %s %w", string(out), err)
	}
	fmt.Printf("  %s\n", strings.TrimSpace(string(out)))
	fmt.Println("Windows service removed.")
	return nil
}
