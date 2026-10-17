package flowgraph

import (
	"strings"
	"time"

	// Embedded zone database, so IANA names resolve on hosts without
	// /usr/share/zoneinfo (static binaries, slim containers).
	_ "time/tzdata"
)

// Location returns the first name that is a valid IANA zone (e.g. the org's
// "Asia/Kolkata" timezone setting), else the server's local zone. Timing
// nodes use it so business hours follow the business, not the host — a
// server in UTC would otherwise judge 09:00–17:00 IST as 14:30–22:30.
func Location(names ...string) *time.Location {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if loc, err := time.LoadLocation(n); err == nil {
			return loc
		}
	}
	return time.Local
}
