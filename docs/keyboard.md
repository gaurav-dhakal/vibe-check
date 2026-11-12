# Keyboard & mouse

| Key | Action |
|---|---|
| `j` `k` / `↓` `↑` | move the cursor |
| `g` `G` | jump to top / bottom |
| `Ctrl+d` `Ctrl+u` | half page down / up |
| `h` `l` | switch pane (dashboard) · switch tab (everywhere else) |
| `1`–`6` | jump to tab — dashboard · processes · ports · docker · disk · apps |
| `f` or `Space` | zoom the current pane |
| `/` | live fuzzy filter on the current list |
| `Esc` | clear the filter · close an overlay · unzoom |
| `Ctrl+p` | command palette |
| `?` or `F1` | keymap drawer |
| `y` / `Y` | copy row / copy row as JSON, to the native clipboard |
| `x` / `X` | act on the row: SIGTERM / SIGKILL a process · purge an artifact (asks first) · stop or start a container · uninstall an app / uninstall **and** delete its files |
| `o` | orphan-only filter |
| `s` | cycle sort order |
| `r` | rescan disk artifacts · re-read the package managers (apps tab) |
| `t` | cycle theme, saved back to config |
| `q` / `Ctrl+c` | quit — the terminal is always restored |

## Dashboard · reclaim keys

| Key | Action |
|---|---|
| `c` | boost & clean sweep: kill orphans, purge the junk list, drop the page cache — after listing what it will do |
| `C` | toggle auto-clean (unattended purge of build junk ≥ `auto_clean_junk_mb`), saved to config |
| `b` | drop the kernel page cache so free RAM is honest (elevated; Linux/macOS) |

## Apps tab

| Key | Action |
|---|---|
| `⏎` | browse the selected app's files: every path it owns, per-folder counts and sizes, totals |
| `i` | install from a local file (`.deb` · `.rpm` · `.tar.gz` · `.tar.xz` · `.AppImage`, per-platform) |
| `Ctrl+f` | in the install box: open the desktop's own file picker — the XDG portal on Linux (what a browser uses), `osascript` on macOS, the shell dialog on Windows; `zenity`/`kdialog` if there is no portal |
| `x` / `X` | uninstall, keeping config/data · uninstall and delete its files |
| `r` | re-read every detected package manager |
| `s` | sort by name → size → manager |

Inside the file browser: `j` `k` `g` `G` `Ctrl+d` `Ctrl+u` to scroll, `y` / `Y` to copy a path (or the
whole listing), `x` / `X` to uninstall with or without its files, `Esc` to close.

## Small terminals

Nothing hides at a size you can actually use. Below ~70 columns the key bar wraps to a second row
instead of truncating, and the tab strip drops its counts, then shortens its labels
(`dash procs ports dock disk apps`), then gives its row back to the list. Modal overlays — help,
palette, file browser, installer, confirmation — cap themselves to the window and scroll, so the
close hint stays on screen. Lists drop their right-most columns (`INFO`, then `VERSION`) rather than
wrap mid-row, and the dashboard collapses from three panes across to two and then one. Under 12×5
you get a one-line "widen the terminal" message with the size it saw, never a blank screen.

Mouse works as expected: click tabs, rows and panes; wheel to scroll.
