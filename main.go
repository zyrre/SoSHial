package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
	gossh "golang.org/x/crypto/ssh"
)

const (
	dbPath = "./soshial.db"
)

// detectColorProfile intelligently detects the color profile based on TERM environment
func detectColorProfile(term string) termenv.Profile {
	term = strings.ToLower(term)

	// TrueColor terminals (24-bit color)
	// Modern terminals like Alacritty, Kitty, WezTerm, etc.
	if strings.Contains(term, "truecolor") ||
		strings.Contains(term, "24bit") ||
		strings.Contains(term, "alacritty") ||
		strings.Contains(term, "kitty") ||
		strings.Contains(term, "wezterm") ||
		strings.HasPrefix(term, "iterm") ||
		strings.HasSuffix(term, "-direct") {
		return termenv.TrueColor
	}

	// Most modern terminals report as xterm-256color but actually support TrueColor
	// This includes: Alacritty, Kitty, modern GNOME Terminal, Konsole, etc.
	// TrueColor colors gracefully degrade to 256 colors if not supported
	if strings.Contains(term, "256color") || strings.Contains(term, "256") {
		return termenv.TrueColor
	}

	// Screen/tmux - these are terminal multiplexers that typically support 256 colors
	// But may not support TrueColor depending on configuration
	if strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return termenv.ANSI256
	}

	// Basic xterm without 256color suffix
	if strings.HasPrefix(term, "xterm") {
		return termenv.ANSI256
	}

	// Basic ANSI terminals (16 colors)
	// Windows cmd, older terminals
	if strings.Contains(term, "ansi") ||
		strings.Contains(term, "cygwin") ||
		strings.Contains(term, "msys") ||
		term == "dumb" {
		return termenv.ANSI
	}

	// Default to TrueColor for unknown modern terminals
	// Most SSH clients in 2025+ support TrueColor
	return termenv.TrueColor
}

// profileName returns a human-readable name for the color profile
func profileName(p termenv.Profile) string {
	switch p {
	case termenv.TrueColor:
		return "TrueColor (24-bit)"
	case termenv.ANSI256:
		return "ANSI256 (256 colors)"
	case termenv.ANSI:
		return "ANSI (16 colors)"
	case termenv.Ascii:
		return "ASCII (no colors)"
	default:
		return "Unknown"
	}
}

func main() {
	// Get host from environment variable, default to localhost
	host := os.Getenv("SOSHIAL_HOST")
	if host == "" {
		host = "localhost"
	}

	// Get port from environment variable, default to 2222
	port := 2222
	if portEnv := os.Getenv("SOSHIAL_PORT"); portEnv != "" {
		if p, err := strconv.Atoi(portEnv); err == nil {
			port = p
		}
	}

	db, err := NewDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Create rate limiter and connection tracker
	rateLimiter := NewRateLimiter(10 * time.Second)
	connTracker := NewConnectionTracker(5) // Max 5 connections per user

	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%d", host, port)),
		wish.WithHostKeyPath(".ssh/soshial_host_key"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			// Accept all public keys
			return true
		}),
		wish.WithMiddleware(
			bubbleTeaMiddleware(db, rateLimiter, connTracker),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting SSH server on %s:%d", host, port)
	log.Printf("Database: %s", dbPath)
	log.Printf("Connect with: ssh %s -p %d", host, port)

	go func() {
		if err = s.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-done
	log.Println("Stopping server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown server: %v", err)
	}
}

// ConnectionTracker tracks active connections per user
type ConnectionTracker struct {
	activeConnections map[string]int
	maxPerUser        int
	mu                sync.Mutex
}

func NewConnectionTracker(maxPerUser int) *ConnectionTracker {
	return &ConnectionTracker{
		activeConnections: make(map[string]int),
		maxPerUser:        maxPerUser,
	}
}

// CanConnect checks if a user can open a new connection
func (ct *ConnectionTracker) CanConnect(userKey string) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	count := ct.activeConnections[userKey]
	return count < ct.maxPerUser
}

// AddConnection increments the connection count for a user
func (ct *ConnectionTracker) AddConnection(userKey string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.activeConnections[userKey]++
}

// RemoveConnection decrements the connection count for a user
func (ct *ConnectionTracker) RemoveConnection(userKey string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if count := ct.activeConnections[userKey]; count > 0 {
		ct.activeConnections[userKey]--
		// Clean up if no more connections
		if ct.activeConnections[userKey] == 0 {
			delete(ct.activeConnections, userKey)
		}
	}
}

// RateLimiter tracks the last message time per user
type RateLimiter struct {
	lastMessageTime map[string]time.Time
	minInterval     time.Duration
	mu              sync.Mutex
}

func NewRateLimiter(minInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		lastMessageTime: make(map[string]time.Time),
		minInterval:     minInterval,
	}

	// Start cleanup goroutine to prevent memory leak
	// Remove entries older than 1 hour every 15 minutes
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// CanSendMessage checks if a user can send a message
func (rl *RateLimiter) CanSendMessage(userKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	lastTime, exists := rl.lastMessageTime[userKey]
	if !exists {
		return true
	}

	return time.Since(lastTime) >= rl.minInterval
}

// RecordMessage records that a user sent a message
func (rl *RateLimiter) RecordMessage(userKey string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.lastMessageTime[userKey] = time.Now()
}

// cleanup removes rate limiter entries older than 1 hour to prevent memory leak
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for key, lastTime := range rl.lastMessageTime {
		if lastTime.Before(cutoff) {
			delete(rl.lastMessageTime, key)
		}
	}
}

func bubbleTeaMiddleware(db *Database, rateLimiter *RateLimiter, connTracker *ConnectionTracker) wish.Middleware {
	teaHandler := func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		pty, _, active := s.Pty()
		if !active {
			wish.Fatalln(s, "no active terminal, skipping")
			return nil, nil
		}

		// Get SSH public key fingerprint
		pubKey := s.PublicKey()
		if pubKey == nil {
			wish.Fatalln(s, "no public key found")
			return nil, nil
		}
		// Get fingerprint and strip "SHA256:" prefix
		fingerprintFull := gossh.FingerprintSHA256(pubKey)
		fingerprint := strings.TrimPrefix(fingerprintFull, "SHA256:")

		// Check connection limit per user
		if !connTracker.CanConnect(fingerprint) {
			wish.Fatalln(s, "connection limit reached (max 5 concurrent connections per user)")
			return nil, nil
		}

		// Track this connection
		connTracker.AddConnection(fingerprint)

		// Ensure connection is removed when session ends
		go func() {
			<-s.Context().Done()
			connTracker.RemoveConnection(fingerprint)
		}()

		// Upsert user in database
		isNewUser, err := db.UpsertUser(fingerprint)
		if err != nil {
			wish.Fatalln(s, fmt.Sprintf("failed to update user: %v", err))
			return nil, nil
		}

		// Send welcome message for new users
		if isNewUser {
			if err := db.SendMessage("system", fingerprint, "Welcome to SoSHial!"); err != nil {
				log.Printf("Failed to send welcome message to %s: %v", fingerprint, err)
			}
		}

		// Create a renderer with color support for the SSH session
		renderer := lipgloss.NewRenderer(s)

		// Auto-detect color profile based on terminal capabilities
		termEnv := pty.Term
		log.Printf("Terminal type: %s", termEnv)

		// Intelligently set color profile based on TERM environment
		profile := detectColorProfile(termEnv)
		renderer.SetColorProfile(profile)
		log.Printf("Using color profile: %s", profileName(profile))

		m := newModel(db, fingerprint, renderer, rateLimiter)
		m.width = pty.Window.Width
		m.height = pty.Window.Height

		return m, []tea.ProgramOption{
			tea.WithAltScreen(),
		}
	}

	return bubbletea.Middleware(teaHandler)
}
