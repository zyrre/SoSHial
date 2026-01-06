package main

import (
	"regexp"
	"strings"
)

var (
	// Regex to match ANSI escape sequences (colors, cursor control, etc.)
	ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

	// Regex to match control characters except newline (\n) and tab (\t)
	controlRegex = regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)

	// Regex to validate SSH fingerprint format (43 chars of base64)
	fingerprintRegex = regexp.MustCompile(`^[A-Za-z0-9+/]{43}$`)
)

// SanitizeMessage removes ANSI escape codes and dangerous control characters
// from message content to prevent terminal manipulation attacks.
func SanitizeMessage(input string) string {
	// Remove ANSI escape sequences
	cleaned := ansiRegex.ReplaceAllString(input, "")

	// Remove control characters except newline and tab
	cleaned = controlRegex.ReplaceAllString(cleaned, "")

	return strings.TrimSpace(cleaned)
}

// ValidateSSHFingerprint checks if a fingerprint string is in valid format.
// SSH-256 fingerprints (without "SHA256:" prefix) are 43 characters of base64.
func ValidateSSHFingerprint(fp string) bool {
	if len(fp) != 43 {
		return false
	}
	return fingerprintRegex.MatchString(fp)
}
