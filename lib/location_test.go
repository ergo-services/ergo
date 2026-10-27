package lib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAfterZoneinfo(t *testing.T) {
	cases := []struct {
		path string
		name string
		ok   bool
	}{
		{"/usr/share/zoneinfo/Europe/Moscow", "Europe/Moscow", true},
		{"/var/db/timezone/zoneinfo/America/New_York", "America/New_York", true},
		{"/usr/share/zoneinfo.default/Asia/Tokyo", "Asia/Tokyo", true},
		{"../usr/share/zoneinfo/posix/Europe/Berlin", "Europe/Berlin", true},
		{"/usr/share/zoneinfo/UTC", "UTC", true},
		{"/usr/share/zoneinfo/", "", false},
		{"/etc/localtime", "", false},
	}
	for _, c := range cases {
		name, ok := afterZoneinfo(c.path)
		if name != c.name || ok != c.ok {
			t.Fatalf("%s: got %q %v, want %q %v", c.path, name, ok, c.name, c.ok)
		}
	}
}

// The local zone read from a link: its name is the one the link goes to.
func TestZoneinfoNameBehindLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no symlinks to zoneinfo on windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink("/usr/share/zoneinfo/Europe/Moscow", link); err != nil {
		t.Fatal(err)
	}
	if name, ok := zoneinfoName(link); name != "Europe/Moscow" || ok == false {
		t.Fatalf("got %q %v", name, ok)
	}
	file := filepath.Join(dir, "copied")
	if err := os.WriteFile(file, []byte("TZif"), 0o644); err != nil {
		t.Fatal(err)
	}
	if name, ok := zoneinfoName(file); ok {
		t.Fatalf("a copied file named %q", name)
	}
}

func TestLocationName(t *testing.T) {
	if name := LocationName(time.UTC); name != "UTC" {
		t.Fatalf("UTC: %q", name)
	}
	if name := LocationName(time.FixedZone("X", 3600)); name != "X" {
		t.Fatalf("fixed: %q", name)
	}
	if ny, err := time.LoadLocation("America/New_York"); err == nil {
		if name := LocationName(ny); name != "America/New_York" {
			t.Fatalf("New York: %q", name)
		}
	}
	if name := LocationName(time.Local); name == "" || name == "Local" {
		t.Fatalf("local: %q", name)
	}
}
