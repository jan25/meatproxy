// Package ui prints numbered pick lists and reads a choice.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Pickable is one row of a pick list.
type Pickable interface {
	Label() string
}

// Pick shows a numbered list and returns the chosen index, or -1 if the user
// cancels. Invalid input is re-prompted up to three times.
func Pick(items []Pickable) int {
	if len(items) == 0 {
		return -1
	}
	for i, it := range items {
		Printf("  %2d. %s\n", i+1, it.Label())
	}

	reader := bufio.NewReader(os.Stdin)
	for attempt := 0; attempt < 3; attempt++ {
		Println("number (enter to cancel):")
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return -1
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return -1
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(items) {
			return n - 1
		}
		if attempt < 2 {
			Printf("invalid choice (%q); try again\n", line)
		}
	}
	return -1
}

// ShortRelTime renders an age like "2h ago" for list rows.
func ShortRelTime(millis int64) string {
	d := time.Since(time.UnixMilli(millis))
	if d < 0 {
		return "now"
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
