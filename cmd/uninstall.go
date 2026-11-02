package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/spf13/cobra"
)

var uninstallYes bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "remove the vibe-check and vcheck binaries and the config directory",
	RunE:  uninstall,
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallYes, "yes", false, "skip the confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}

func uninstall(*cobra.Command, []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	dir := filepath.Dir(exe)
	lower := strings.ToLower(exe)

	// Package managers own their files; deleting them behind brew's or winget's
	// back leaves a ghost install that update/uninstall can never fix.
	switch {
	case strings.Contains(lower, "cellar") || strings.Contains(lower, "homebrew"):
		fmt.Println("installed via Homebrew — run instead:")
		fmt.Println("  brew uninstall vibe-check && brew untap gaurav-dhakal/vibe-check")
		return nil
	case strings.Contains(lower, "winget"):
		fmt.Println("installed via winget — run instead:")
		fmt.Println("  winget uninstall gaurav-dhakal.vibe-check")
		return nil
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	var targets []string
	for _, name := range []string{"vibe-check" + ext, "vcheck" + ext} {
		if p := filepath.Join(dir, name); fileExists(p) {
			targets = append(targets, p)
		}
	}
	cfgDir := filepath.Dir(config.Path())

	fmt.Println("vibe-check uninstall will remove:")
	for _, t := range targets {
		fmt.Println("  " + t)
	}
	if fileExists(cfgDir) {
		fmt.Println("  " + cfgDir + "  (config)")
	}
	if !uninstallYes {
		if !stdinIsTTY() {
			return fmt.Errorf("refusing to uninstall non-interactively; pass --yes")
		}
		fmt.Print("\nproceed? [y/N] ")
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() || !strings.EqualFold(strings.TrimSpace(sc.Text()), "y") {
			fmt.Println("cancelled — nothing removed")
			return nil
		}
	}

	for _, t := range targets {
		if err := os.Remove(t); err != nil {
			fmt.Printf("could not remove %s: %v\n", t, err)
			switch {
			case os.IsPermission(err):
				fmt.Println("  (no write access — try: sudo " + t + " uninstall --yes)")
			case runtime.GOOS == "windows" && strings.EqualFold(t, exe):
				fmt.Println("  (that is the running binary — close this terminal, or delete it manually)")
			}
			continue
		}
		fmt.Println("removed " + t)
	}
	if fileExists(cfgDir) {
		if err := os.RemoveAll(cfgDir); err == nil {
			fmt.Println("removed " + cfgDir)
		}
	}
	fmt.Println("done — thanks for trying vibe-check")
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
