//go:build linux

package apps

import "testing"

func TestFileFromURI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"file:///home/me/Downloads/some%20app.deb", "/home/me/Downloads/some app.deb"},
		{"file:///home/me/a.deb", "/home/me/a.deb"},
		{"file:///home/me/100%25%23weird.deb", "/home/me/100%#weird.deb"},
		{"http://example.com/a.deb", ""},   // not a local file
		{"", ""},                           // nothing picked
		{"file:///home/me/bad%zz.deb", ""}, // undecodable
	}
	for _, c := range cases {
		if got := fileFromURI(c.in); got != c.want {
			t.Errorf("fileFromURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
