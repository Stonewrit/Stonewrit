package core

import "time"

// HashTime renders a timestamp into the exact textual form used inside the
// payload-hash pre-image. It is millisecond-precision UTC with a literal "Z",
// byte-identical to JavaScript's Date.prototype.toISOString() output, which is
// always YYYY-MM-DDTHH:mm:ss.sssZ.
//
// Millisecond precision is deliberate. Any browser-based auditor recomputes
// these hashes with the JavaScript Date type, which is millisecond resolution
// and cannot represent microseconds. Pinning the pre-image to millisecond UTC
// is what lets every implementation (Go, JavaScript, and future SDKs) agree by
// construction.
//
// HashTime does not read the clock. The caller supplies the time value.
func HashTime(t time.Time) string {
	// Truncate to millisecond so the fractional slot is exact, then append a
	// literal Z (the value is always UTC). Formatting the zone via Go's
	// "Z07:00" token would emit "+00:00" for some inputs; the hardcoded Z
	// removes that ambiguity.
	return t.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000") + "Z"
}
