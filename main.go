package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	gossh "golang.org/x/crypto/ssh"
)

const (
	port   = 2222
	dbPath = "./soshial.db"
)

func main() {
	// Get host from environment variable, default to localhost
	host := os.Getenv("SOSHIAL_HOST")
	if host == "" {
		host = "localhost"
	}

	db, err := NewDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Create rate limiter
	rateLimiter := NewRateLimiter(10 * time.Second)

	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%d", host, port)),
		wish.WithHostKeyPath(".ssh/soshial_host_key"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			// Accept all public keys
			return true
		}),
		wish.WithMiddleware(
			bubbleTeaMiddleware(db, rateLimiter),
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

// RateLimiter tracks the last message time per user
type RateLimiter struct {
	lastMessageTime map[string]time.Time
	minInterval     time.Duration
	mu              sync.Mutex
}

func NewRateLimiter(minInterval time.Duration) *RateLimiter {
	return &RateLimiter{
		lastMessageTime: make(map[string]time.Time),
		minInterval:     minInterval,
	}
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

func bubbleTeaMiddleware(db *Database, rateLimiter *RateLimiter) wish.Middleware {
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

		// Upsert user in database
		if err := db.UpsertUser(fingerprint); err != nil {
			wish.Fatalln(s, fmt.Sprintf("failed to update user: %v", err))
			return nil, nil
		}

		// Create a renderer with color support for the SSH session
		renderer := lipgloss.NewRenderer(s)

		// Force color output - ANSI256 colors (profile ID 2)
		termEnv := pty.Term
		log.Printf("Terminal type: %s", termEnv)

		// Force ANSI256 color profile
		renderer.SetColorProfile(2) // 2 = ANSI256

		m := newModel(db, fingerprint, renderer, rateLimiter)
		m.width = pty.Window.Width
		m.height = pty.Window.Height

		return m, []tea.ProgramOption{
			tea.WithAltScreen(),
		}
	}

	return bubbletea.Middleware(teaHandler)
}
