package engine

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
)

// Copy puts text on the system clipboard. Prefers native helpers when
// present (wl-clipboard > xclip > xsel > pbcopy > termux), else falls
// back to OSC 52 escape codes which most modern terminals honor.
func Copy(text string) (string, error) {
	type tool struct {
		bin  string
		args []string
	}
	tools := []tool{
		{"wl-copy", nil},
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
		{"pbcopy", nil}, // macOS
		{"clip", nil},   // Windows clipboard
		{"termux-clipboard-set", nil},
	}
	for _, t := range tools {
		if path, err := exec.LookPath(t.bin); err == nil {
			cmd := exec.Command(path, t.args...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return t.bin, nil
			}
		}
	}
	if err := osc52(text); err != nil {
		return "", err
	}
	return "osc52", nil
}

func osc52(text string) error {
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	// OSC 52 ; clipboard (c) ; base64 BEL
	_, err := os.Stdout.WriteString("\x1b]52;c;" + b64 + "\x07")
	return err
}
