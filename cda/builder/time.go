// cda/builder/time.go
package builder

import "time"

// nowCDATimestamp returns the current time in CDA's TS format (YYYYMMDDHHMMSS, UTC).
func nowCDATimestamp() string {
	return time.Now().UTC().Format("20060102150405")
}

func timeNowUnixNano() int64 {
	return time.Now().UnixNano()
}
