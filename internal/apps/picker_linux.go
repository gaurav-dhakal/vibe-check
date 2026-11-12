//go:build linux

package apps

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	dbus "github.com/godbus/dbus/v5"
)

// pickFile opens the desktop's own file chooser — the very dialog a browser
// gets. Wayland compositors do not let a plain XUL-less process pop a window
// over someone else's session, so the desktop exposes one shared picker over
// D-Bus (the XDG portal) and every app asks for it. COSMIC, GNOME and KDE all
// answer with their native dialog; zenity and kdialog are the fallbacks for a
// session that runs no portal, and with neither the install flow keeps the
// typed path instead of dying on a window that cannot exist.
func pickFile() (string, error) {
	p, err := portalPick()
	if err == nil || errors.Is(err, ErrPickCancelled) {
		return p, err
	}
	for _, c := range []struct {
		bin  string
		args []string
	}{
		{"zenity", []string{"--file-selection", "--title=vibe-check · install from file"}},
		{"kdialog", []string{"--getopenfilename", homeShort()}},
	} {
		if !hasBin(c.bin) {
			continue
		}
		s, code, err := runStatus(c.bin, c.args...)
		switch {
		case err == nil:
			return s, nil
		case code == 1:
			return "", ErrPickCancelled
		default:
			return "", &CmdError{Name: c.bin, Msg: clipLine(lastNonEmpty(s))}
		}
	}
	return "", &CmdError{Name: "file chooser", Msg: clipLine(lastNonEmpty(err.Error()))}
}

func pickerAvailable() bool {
	if portalReady() {
		return true
	}
	// a classic dialog needs a compositor to draw on
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	return hasBin("zenity") || hasBin("kdialog")
}

const (
	portalName   = "org.freedesktop.portal.Desktop"
	portalPath   = "/org/freedesktop/portal/desktop"
	fileChooser  = "org.freedesktop.portal.FileChooser"
	requestIface = "org.freedesktop.portal.Request"
)

// portalReady is a yes/no on whether the session can produce a picker at all.
// The portal service is usually activatable rather than running, so asking for
// it by name is the only honest check.
func portalReady() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	return startPortal(conn) == nil
}

func startPortal(conn *dbus.Conn) error {
	var r uint32
	return conn.BusObject().Call("org.freedesktop.DBus.StartServiceByName", 0,
		portalName, uint32(0)).Store(&r)
}

// portalPick runs the portal's OpenFile, then waits on the Response signal the
// request object emits when the user answers. The call itself only hands back
// the request path, which is why this is two steps.
func portalPick() (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := startPortal(conn); err != nil {
		return "", err
	}

	call := conn.Object(portalName, dbus.ObjectPath(portalPath)).Call(
		fileChooser+".OpenFile", 0, "",
		"vibe-check · install from file", map[string]dbus.Variant{})
	if call.Err != nil {
		return "", call.Err
	}
	req, ok := call.Body[0].(dbus.ObjectPath)
	if !ok {
		return "", errors.New("the portal returned no request")
	}

	rule := "type='signal',interface=" + requestIface +
		",member='Response',path=" + string(req)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return "", err
	}
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	// the shared session connection outlives one dialog: the next `i` presses
	// reuse it, so only the signal subscription is temporary
	defer conn.RemoveSignal(ch)

	// the dialog belongs to the desktop, not to us: it can be left open while
	// the user does something else, so give it plenty of room but never forever
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case s, open := <-ch:
			if !open {
				return "", errors.New("the desktop connection closed")
			}
			if s.Path != req || len(s.Body) < 2 {
				continue
			}
			code, _ := s.Body[0].(uint32)
			switch code {
			case 0: // accepted
			case 1:
				return "", ErrPickCancelled
			default:
				return "", errors.New("the desktop refused to open a file dialog")
			}
			results, _ := s.Body[1].(map[string]dbus.Variant)
			var uris []string
			if v, ok := results["uris"]; ok {
				_ = v.Store(&uris)
			}
			if len(uris) == 0 {
				return "", ErrPickCancelled
			}
			return fileFromURI(uris[0]), nil
		case <-timer.C:
			return "", errors.New("the file dialog never answered")
		}
	}
}

// fileFromURI turns portal output ("file:///home/me/a.deb", percent-encoded)
// back into a path. Anything that is not a local file is refused rather than
// handed to an installer that would read it as a relative path.
func fileFromURI(u string) string {
	if !strings.HasPrefix(u, "file:") {
		return ""
	}
	p, err := url.PathUnescape(strings.TrimPrefix(u, "file://"))
	if err != nil {
		return ""
	}
	return p
}
