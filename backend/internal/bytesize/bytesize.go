// Package bytesize formats byte counts for human-readable messages.
package bytesize

import "fmt"

// Format uses binary units like the panel's subscription pages.
func Format(bytes int64) string {
	bytes = max(bytes, 0)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
