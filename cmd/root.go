package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xterm "github.com/charmbracelet/x/term"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
	"github.com/gaurav-dhakal/vibe-check/internal/ui"
	"github.com/spf13/cobra"
)

var (
	cfgPath       string
	themeFlag     string
	interval      int
	jsonOut       bool
	noMouse       bool
	useDefaults   bool
	uninstallFlag bool
	version       = "0.1.3"
)

var rootCmd = &cobra.Command{
	Use:           "vibe-check",
	Aliases:       []string{"vcheck"},
	Short:         "live local dev-machine telemetry: processes, ports, docker, disk",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          run,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "manage ~/.config/vibe-check/config.toml",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "write default config (if none) and print its path",
	RunE: func(*cobra.Command, []string) error {
		p, err := config.Init()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, p)
		return nil
	},
}

func init() {
	rootCmd.Flags().StringVar(&cfgPath, "config", "", "path to config.toml")
	rootCmd.Flags().StringVar(&themeFlag, "theme", "",
		"one of: "+strings.Join(theme.Names(), " | "))
	rootCmd.Flags().IntVar(&interval, "interval", 0, "sys telemetry interval in ms (250-1000)")
	rootCmd.Flags().BoolVar(&jsonOut, "json", false, "print one snapshot as JSON and exit (no TUI, pipe-friendly)")
	rootCmd.Flags().BoolVar(&noMouse, "no-mouse", false, "disable terminal mouse tracking")
	rootCmd.Flags().BoolVar(&useDefaults, "default", false, "skip the first-run theme prompt and keep the defaults")
	rootCmd.Flags().BoolVar(&uninstallFlag, "uninstall", false, "remove the binary and config (same as: vibe-check uninstall)")
	// Bound to the same variable as `vibe-check uninstall --yes`, so either
	// spelling can run unattended.
	rootCmd.Flags().BoolVar(&uninstallYes, "yes", false, "with --uninstall: skip the confirmation")
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}

func Execute() {
	defer restoreTerminal()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "vibe-check:", err)
		os.Exit(1)
	}
}

// restoreTerminal puts the shell back exactly as we found it even if we
// died mid-frame: leave alt screen, show cursor, reset attributes.
// Written to /dev/tty so piped stdout (e.g. --json) stays pure.
func restoreTerminal() {
	seq := "\x1b[?1049l\x1b[?25h\x1b[0m"
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err == nil {
		tty.WriteString(seq)
		tty.Close()
		return
	}
	// No controlling terminal (detached pty, some CI runners): the TUI's own
	// output stream is the terminal, and these codes are idempotent.
	if fi, e := os.Stdout.Stat(); e == nil && fi.Mode()&os.ModeCharDevice != 0 {
		os.Stdout.WriteString(seq)
	}
}

func run(cmd *cobra.Command, args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			restoreTerminal()
			err = fmt.Errorf("panic recovered, terminal restored: %v", r)
		}
	}()

	// `vibe-check --uninstall` is the same door as `vibe-check uninstall`.
	if uninstallFlag {
		return uninstall(cmd, args)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config %s: %v", cfgPath, err)
	}
	if interval > 0 {
		cfg.SysIntervalMS = clampInt(interval, 250, 1000)
		cfg.ProcIntervalMS = clampInt(interval*2, 250, 5000)
	}
	if themeFlag != "" {
		if !theme.Exists(themeFlag) {
			return fmt.Errorf("unknown theme %q (try %v)", themeFlag, theme.Names())
		}
		cfg.Theme = themeFlag
	}

	// First run on a real terminal: ask for a theme instead of silently
	// starting with catppuccin. --json and --theme skip the question;
	// --default takes the defaults and records them so nothing is asked twice.
	switch {
	case jsonOut || themeFlag != "":
	case useDefaults:
		config.Init() // remember the defaults, so the next launch stays quiet
	case configMissing(cfgPath) && stdinIsTTY():
		if picked := promptTheme(os.Stdout, os.Stdin); picked != "" {
			cfg.Theme = picked
			if p, err := config.InitWithTheme(picked); err == nil {
				fmt.Println("config written to", p)
			}
		}
	}

	th := theme.Default(cfg.Theme)
	eng := engine.New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// a piped report wants complete data now; only the TUI crawls politely
	if jsonOut {
		eng.ScanFullSpeed()
	}
	eng.Run(ctx)

	if jsonOut {
		return runHeadless(eng, ctx)
	}

	m := ui.New(eng, cfg, th, cfg.Theme)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.Mouse && !noMouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	prog := tea.NewProgram(m, opts...)

	go func() {
		for ev := range eng.Events() {
			prog.Send(ui.EventMsg(ev))
		}
	}()

	// Ctrl+C arrives as a key in raw mode, but kill -2 is a real signal: treat
	// it like a quit so the shell is never left in a broken state.
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, exitSignals()...)
	exited := make(chan struct{})
	go func() {
		for range sigCh {
			signal.Stop(sigCh)
			// Quit lets the renderer undo raw mode and leave the alt screen;
			// the deadline only covers a wedged render loop.
			go prog.Quit()
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
				restoreTerminal()
				os.Exit(0)
			}
			return
		}
	}()

	final, err := prog.Run()
	close(exited)
	// SIGINT and SIGTERM both mean "get out", and bubbletea reports them as
	// errors; only a real failure should reach the shell as a nonzero exit.
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	_ = final
	return nil
}

func runHeadless(eng *engine.Engine, ctx context.Context) error {
	start := time.Now()
	deadline := start.Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s := eng.Snapshot()
		// wait for one full cycle of every fast loop + the first disk scan
		if len(s.Procs) > 0 && s.Sys.Hostname != "" &&
			!s.ScanState.Scanning && !s.ScanState.LastDoneAt.IsZero() &&
			time.Since(start) > 1500*time.Millisecond {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(s)
		}
		time.Sleep(200 * time.Millisecond)
	}
	s := eng.Snapshot()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func configMissing(explicit string) bool {
	p := explicit
	if p == "" {
		p = config.Path()
	}
	_, err := os.Stat(p)
	return err != nil
}

// stdinIsTTY guards every prompt: /dev/null and pipes are character devices
// too, so the stat check alone would ask questions nobody can answer.
func stdinIsTTY() bool {
	return xterm.IsTerminal(uintptr(os.Stdin.Fd()))
}

func promptTheme(w io.Writer, in io.Reader) string {
	names := theme.Names()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  \x1b[1;35m● vibe-check\x1b[0m — first run")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  pick a theme:")
	for i, n := range names {
		suffix := ""
		if i == 0 {
			suffix = "  \x1b[2m(default)\x1b[0m"
		}
		fmt.Fprintf(w, "    \x1b[1m%d\x1b[0m) %-11s \x1b[2m%s\x1b[0m%s\n",
			i+1, n, theme.DisplayName(n), suffix)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  \x1b[2mpress t inside the app to cycle themes; this choice is saved to your config\x1b[0m")
	fmt.Fprint(w, "\n  number + enter, or just enter for the default: ")

	// A closed stdin means nobody is there to answer; leave the config alone.
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(w)
		return ""
	}
	choice := strings.TrimSpace(sc.Text())
	if choice == "" {
		return names[0]
	}
	if theme.Exists(choice) {
		return choice
	}
	if idx, err := strconv.Atoi(choice); err == nil && idx >= 1 && idx <= len(names) {
		return names[idx-1]
	}
	fmt.Fprintln(w, "  unknown choice — using", names[0])
	return names[0]
}
