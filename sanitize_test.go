package main

import (
	"testing"
)

func TestSanitizeMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal message",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "Message with ANSI color codes",
			input:    "\x1b[31mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "Message with clear screen",
			input:    "\x1b[2J\x1b[HCleared screen",
			expected: "Cleared screen",
		},
		{
			name:     "Message with control characters",
			input:    "Hello\x00\x01\x02World",
			expected: "HelloWorld",
		},
		{
			name:     "Message with newlines and tabs (should keep)",
			input:    "Line 1\nLine 2\tTabbed",
			expected: "Line 1\nLine 2\tTabbed",
		},
		{
			name:     "Empty message",
			input:    "",
			expected: "",
		},
		{
			name:     "Only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "Complex ANSI attack",
			input:    "\x1b[2J\x1b[H\x1b[31mFAKE ERROR\x1b[0m\x07",
			expected: "FAKE ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeMessage(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeMessage(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateSSHFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
		valid       bool
	}{
		{
			name:        "Valid fingerprint",
			fingerprint: "AAAAB3NzaC1yc2EAAAADAQABAAABAQC1234567890+/",
			valid:       true,
		},
		{
			name:        "Too short",
			fingerprint: "AAAAB3NzaC1yc2EAAAADAQAB",
			valid:       false,
		},
		{
			name:        "Too long",
			fingerprint: "AAAAB3NzaC1yc2EAAAADAQABAAABAQC1234567890+/EXTRA",
			valid:       false,
		},
		{
			name:        "Invalid characters",
			fingerprint: "AAAAB3NzaC1yc2EAAAADAQABAAABAQC123456789!@",
			valid:       false,
		},
		{
			name:        "Empty string",
			fingerprint: "",
			valid:       false,
		},
		{
			name:        "With SHA256 prefix (should be invalid - prefix should be stripped before validation)",
			fingerprint: "SHA256:AAAAB3NzaC1yc2EAAAADAQABAAABAQC1234",
			valid:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSSHFingerprint(tt.fingerprint)
			if result != tt.valid {
				t.Errorf("ValidateSSHFingerprint(%q) = %v; want %v", tt.fingerprint, result, tt.valid)
			}
		})
	}
}
