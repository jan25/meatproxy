package ui

import "fmt"

// Logo is the tool's mark, prefixed to user-facing output lines.
const Logo = "🥩"

// Printf prints a formatted line with the logo prefix.
func Printf(format string, args ...any) {
	fmt.Print(Logo, " ")
	fmt.Printf(format, args...)
}

// Println prints a line with the logo prefix.
func Println(args ...any) {
	fmt.Print(Logo, " ")
	fmt.Println(args...)
}
