package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
	"github.com/gaurav-dhakal/vibe-check/internal/ui"
	"github.com/spf13/cobra"
)

var (
	cfgPath   string
	themeFlag string
	interval  int
	jsonOut   bool
	noMouse   bool
	version   = "0.1.3"
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
	rootCmd.Flags().StringVar(&themeFlag, "theme", "", "catppuccin | tokyonight | gruvbox")
	rootCmd.Flags().IntVar(&interval, "interval", 0, "sys telemetry interval in ms (250-1000)")
	rootCmd.Flags().BoolVar(&jsonOut, "json", false, "print one snapshot as JSON and exit (no TUI, pipe-friendly)")
	rootCmd.Flags().BoolVar(&noMouse, "no-mouse", false, "disable terminal mouse tracking")
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
