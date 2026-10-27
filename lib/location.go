package lib

import (
	"os"
	"strings"
	"time"
)

// LocationName is the name of a location as a node reports it: the IANA
// name (Europe/Moscow), for the local one too - found from the link
// /etc/localtime it is read from, or the path TZ names; the abbreviation
// of its zone now (MSK) if the system does not tell the name.
func LocationName(loc *time.Location) string {
	name := loc.String()
	var found string
	var ok bool
	switch {
	case name == "Local":
		found, ok = zoneinfoName("/etc/localtime")
	case strings.HasPrefix(name, "/"):
		found, ok = zoneinfoName(name)
	default:
		return name
	}
	if ok {
		return found
	}
	abbr, _ := time.Now().In(loc).Zone()
	return abbr
}

// zoneinfoName is the name of the zone a tzdata file is, by its path or
// the path it links to.
func zoneinfoName(path string) (string, bool) {
	if name, ok := afterZoneinfo(path); ok {
		return name, true
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return afterZoneinfo(target)
}

// afterZoneinfo is what goes after the directory of the zones in a path
// (/usr/share/zoneinfo/Europe/Moscow, /var/db/timezone/zoneinfo/Europe/Moscow).
func afterZoneinfo(path string) (string, bool) {
	at := strings.Index(path, "/zoneinfo")
	if at < 0 {
		return "", false
	}
	rest := path[at+len("/zoneinfo"):]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return "", false
	}
	rest = rest[slash+1:]
	if r, ok := strings.CutPrefix(rest, "posix/"); ok {
		rest = r
	} else if r, ok := strings.CutPrefix(rest, "right/"); ok {
		rest = r
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}
