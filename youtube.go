package main

import (
	"fmt"
	"time"
)

// Format duration in a way YouTube understands.
func fmtDurationForYouTube(d time.Duration) string {
	num_hours := int64(d.Hours())
	num_minutes := int64(d.Minutes())
	num_seconds := int64(d.Seconds())
	return fmt.Sprintf("%01d:%02d:%02d", num_hours, num_minutes-60*num_hours, num_seconds-60*num_minutes)
}
