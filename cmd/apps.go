package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gaurav-dhakal/vibe-check/internal/apps"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/spf13/cobra"
)

// Every action the Apps tab has a key for, as a command: the same inventory,
// the same file listing, the same uninstall and install-from-file, no TUI and
// no terminal size to fight.

var (
	appsFilter string
	appsPurge  bool
	appsYes    bool
	appsJSON   bool
)

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "list, inspect, install and remove applications without the TUI",
}

var appsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "every installed app: apt, dnf, pacman, snap, flatpak, AppImage, npm",
	RunE: func(*cobra.Command, []string) error {
		list, err := apps.Load()
		if err != nil {
			return err
		}
		list = filterPkgs(list, appsFilter)
		if appsJSON {
			return encode(list)
		}
		if len(list) == 0 {
			fmt.Println("no apps match", fmt.Sprintf("%q", appsFilter))
			return nil
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Manager != list[j].Manager {
				return list[i].Manager < list[j].Manager
			}
			return list[i].Name < list[j].Name
		})
		for _, p := range list {
			size := "—"
			if p.SizeKB > 0 {
				size = engine.Bytes(uint64(p.SizeKB) * 1024)
			}
			fmt.Printf("%-9s %-28s %-18s %8s  %s\n", p.Manager, cut(p.Name, 28), cut(p.Version, 18), size, p.Desc)
		}
		fmt.Printf("\n%d apps · managers: %s\n", len(list), strings.Join(apps.Available(), ", "))
		return nil
	},
}

var appsFilesCmd = &cobra.Command{
	Use:   "files <name>",
	Short: "files and folders an installed app put on disk",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		p, err := findApp(args[0])
		if err != nil {
			return err
		}
		fl, err := apps.Files(p)
		if err != nil {
			return err
		}
		if appsJSON {
			return encode(fl)
		}
		fmt.Printf("%s (%s %s)\n", p.Name, p.Manager, p.Version)
		fmt.Printf("%d files · %d folders · %s\n\n", fl.Files, fl.Dirs, engine.Bytes(fl.Bytes))
		for _, path := range fl.Paths {
			fmt.Println(path)
		}
		if fl.Truncated {
			fmt.Println("… list capped")
		}
		return nil
	},
}

var appsUninstallCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"uninstall"},
	Short:   "remove an app through its own manager (elevated when needed)",
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		p, err := findApp(args[0])
		if err != nil {
			return err
		}
		what := p.Name + " (" + p.Manager + ")"
		if appsPurge {
			what += " + its files"
		}
		if !appsYes {
			return fmt.Errorf("refusing to remove %s without --yes", what)
		}
		if err := apps.Uninstall(p, appsPurge); err != nil {
			return err
		}
		fmt.Println("✔ removed", what)
		return nil
	},
}

var appsInstallCmd = &cobra.Command{
	Use:   "install <file>",
	Short: "install from a downloaded file: .deb .rpm .tar.gz/.xz .AppImage",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		desc, err := apps.Install(args[0])
		if err != nil {
			return err
		}
		fmt.Println("✔", desc)
		return nil
	},
}

var appsManagersCmd = &cobra.Command{
	Use:   "managers",
	Short: "which package managers this machine reports",
	RunE: func(*cobra.Command, []string) error {
		for _, m := range apps.Available() {
			fmt.Println(m)
		}
		return nil
	},
}

var boostCmd = &cobra.Command{
	Use:   "boost",
	Short: "drop the OS page cache so free RAM is honest (elevated when needed)",
	RunE: func(*cobra.Command, []string) error {
		before, err := mem.VirtualMemory()
		if err != nil {
			return err
		}
		if err := apps.DropPageCache(); err != nil {
			return err
		}
		after, err := mem.VirtualMemory()
		if err != nil {
			return err
		}
		gain := uint64(0)
		if after.Available > before.Available {
			gain = after.Available - before.Available
		}
		fmt.Printf("free %s → %s (+%s)\n", engine.Bytes(before.Available),
			engine.Bytes(after.Available), engine.Bytes(gain))
		return nil
	},
}

var autocleanCmd = &cobra.Command{
	Use:   "autoclean [on|off]",
	Short: "show or set unattended junk purge in the config",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		p := cfgPath
		if p == "" {
			p = config.Path()
		}
		if len(args) == 0 {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			fmt.Printf("%s (junk dirs ≥ %dMB) · config %s\n",
				map[bool]string{true: "on", false: "off"}[cfg.AutoClean], cfg.AutoCleanJunkMB, p)
			return nil
		}
		on, err := parseOnOff(args[0])
		if err != nil {
			return err
		}
		value := "false"
		if on {
			value = "true"
		}
		if err := config.Set(p, "auto_clean", value); err != nil {
			return err
		}
		fmt.Printf("auto_clean = %s in %s\n", value, p)
		return nil
	},
}

// parseOnOff reads the only two states the setting has, plus the usual spellings.
func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "1", "yes":
		return true, nil
	case "off", "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("want on or off, got %q", s)
}

// findApp resolves a name the way the TUI's cursor does: exact match wins,
// otherwise a unique substring; anything ambiguous is an error, never a guess.
func findApp(name string) (apps.Package, error) {
	list, err := apps.Load()
	if err != nil {
		return apps.Package{}, err
	}
	var exact, part []apps.Package
	for _, p := range list {
		if p.Name == name {
			exact = append(exact, p)
		} else if strings.Contains(strings.ToLower(p.Name), strings.ToLower(name)) {
			part = append(part, p)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return apps.Package{}, ambiguous(exact, name)
	}
	if len(part) == 1 {
		return part[0], nil
	}
	if len(part) == 0 {
		return apps.Package{}, fmt.Errorf("no installed app matches %q — try: vibe-check apps list", name)
	}
	return apps.Package{}, ambiguous(part, name)
}

func ambiguous(list []apps.Package, name string) error {
	var b strings.Builder
	for i, p := range list {
		if i == 6 {
			b.WriteString(", …")
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%s)", p.Name, p.Manager)
	}
	return fmt.Errorf("%q matches %d apps: %s — pass the exact name", name, len(list), b.String())
}

func filterPkgs(list []apps.Package, q string) []apps.Package {
	if q == "" {
		return list
	}
	q = strings.ToLower(q)
	out := make([]apps.Package, 0, len(list))
	for _, p := range list {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Desc+" "+p.Manager), q) {
			out = append(out, p)
		}
	}
	return out
}

// cut keeps a fixed-width column honest: apt versions can run 40 characters.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func encode(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	for _, c := range []*cobra.Command{appsListCmd, appsFilesCmd, appsUninstallCmd} {
		c.Flags().BoolVar(&appsJSON, "json", false, "machine-readable output")
	}
	appsListCmd.Flags().StringVar(&appsFilter, "filter", "", "name, manager or description substring")
	appsUninstallCmd.Flags().BoolVar(&appsPurge, "purge", false, "also delete the app's files and settings")
	appsUninstallCmd.Flags().BoolVar(&appsYes, "yes", false, "required: actually run the removal")
	appsCmd.AddCommand(appsListCmd, appsFilesCmd, appsUninstallCmd, appsInstallCmd, appsManagersCmd)
	rootCmd.AddCommand(appsCmd, boostCmd, autocleanCmd)
}
