package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	mainMenu screen = iota
	viewMessages
	sendMessageRecipient
	sendMessageContent
)

type viewMode int

const (
	viewModeThreaded viewMode = iota
	viewModeConversation
)

type themeName string

const (
	themeGruvbox themeName = "gruvbox"
	themeDracula themeName = "dracula"
)

type theme struct {
	name       string
	background lipgloss.Color
	foreground lipgloss.Color
	primary    lipgloss.Color
	secondary  lipgloss.Color
	accent     lipgloss.Color
	success    lipgloss.Color
	error      lipgloss.Color
	muted      lipgloss.Color
	highlight  lipgloss.Color
	selection  lipgloss.Color
}

var themes = map[themeName]theme{
	themeGruvbox: {
		name:       "Gruvbox",
		background: lipgloss.Color("#282828"),
		foreground: lipgloss.Color("#FBF1C7"), // fg0 (bright/white)
		primary:    lipgloss.Color("#FABD2F"), // yellow (bright)
		secondary:  lipgloss.Color("#D3869B"), // purple (bright)
		accent:     lipgloss.Color("#8EC07C"), // aqua (bright)
		success:    lipgloss.Color("#B8BB26"), // green (bright)
		error:      lipgloss.Color("#FB4934"), // red (bright)
		muted:      lipgloss.Color("#A89984"), // gray (brighter)
		highlight:  lipgloss.Color("#FE8019"), // orange (bright)
		selection:  lipgloss.Color("#83A598"), // blue (bright)
	},
	themeDracula: {
		name:       "Dracula",
		background: lipgloss.Color("#282a36"),
		foreground: lipgloss.Color("#f8f8f2"),
		primary:    lipgloss.Color("#bd93f9"), // purple
		secondary:  lipgloss.Color("#ff79c6"), // pink
		accent:     lipgloss.Color("#8be9fd"), // cyan
		success:    lipgloss.Color("#50fa7b"), // green
		error:      lipgloss.Color("#ff5555"), // red
		muted:      lipgloss.Color("#6272a4"), // comment
		highlight:  lipgloss.Color("#ffb86c"), // orange
		selection:  lipgloss.Color("#8be9fd"), // cyan
	},
}

type messageThread struct {
	senderKey     string
	messageCount  int
	latestMessage Message
	allMessages   []Message
}

type model struct {
	db               *Database
	userKey          string
	currentScreen    screen
	renderer         *lipgloss.Renderer
	currentTheme     themeName
	selectedMenuItem int // 0=view, 1=send, 2=theme, 3=quit
	rateLimiter      *RateLimiter

	// For sending messages
	recipientInput textinput.Model
	messageInput   *textarea.Model
	recipient      string

	// For viewing messages
	messages              []Message
	selectedMessageIndex  int
	messageCount          int // Cached count of messages
	messageScrollOffset   int // Current scroll offset for the selected message
	replyMode             bool // Whether reply input is active
	replyInput            textinput.Model

	// For threading
	currentViewMode       viewMode
	threads               []messageThread
	selectedThreadIndex   int
	conversationPartner   string
	conversationMessages  []Message

	// General
	err           error
	successMsg    string
	width         int
	height        int
	clipboardText string // Text to copy to clipboard on next render
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type tickMsg time.Time

var (
	// Color palette
	primaryColor   = lipgloss.Color("#7D56F4")
	secondaryColor = lipgloss.Color("#FF06B7")
	accentColor    = lipgloss.Color("#00D9FF")
	successColor   = lipgloss.Color("#04B575")
	errorColor     = lipgloss.Color("#FF4757")
	textColor      = lipgloss.Color("#FAFAFA")
	mutedColor     = lipgloss.Color("#6C6C6C")
	bgColor        = lipgloss.Color("#1A1A1A")

	// Title with gradient effect
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(bgColor).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(primaryColor).
			Align(lipgloss.Center)

	// Subtitle style
	subtitleStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Italic(true).
			MarginBottom(1)

	// User ID badge
	userIDStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Background(primaryColor).
			Padding(0, 1).
			MarginBottom(1).
			Bold(true)

	// Menu items
	menuItemStyle = lipgloss.NewStyle().
			Foreground(textColor).
			PaddingLeft(2).
			MarginBottom(1)

	menuIconStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	// Success and error messages
	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Background(lipgloss.Color("#0F3D2E")).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(successColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Background(lipgloss.Color("#3D0F1A")).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorColor)

	// Message display
	messageBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			MarginBottom(1).
			Width(70)

	messageHeaderStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true).
				MarginBottom(1)

	messageTimeStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Italic(true)

	messageContentStyle = lipgloss.NewStyle().
				Foreground(textColor).
				MarginTop(1)

	newBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(accentColor).
			Padding(0, 1).
			Bold(true)

	// Input field containers
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			MarginBottom(1)

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			MarginBottom(1)

	// Help text
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(2).
			Italic(true)

	// Divider
	dividerStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			MarginTop(1).
			MarginBottom(1)

	// Empty state
	emptyStateStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Align(lipgloss.Center).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			BorderStyle(lipgloss.Border{
			Top:         "─",
			Bottom:      "─",
			Left:        "│",
			Right:       "│",
			TopLeft:     "╭",
			TopRight:    "╮",
			BottomLeft:  "╰",
			BottomRight: "╯",
		}).
		Padding(2, 4)
)

func newModel(db *Database, userKey string, renderer *lipgloss.Renderer, rateLimiter *RateLimiter) model {
	ti := textinput.New()
	ti.Placeholder = "SSH key (example: nThbg6kXUpJWGl7E1IGOCspRomTxdCARLviKw6E5SY8)"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 80

	ta := textarea.New()
	ta.Placeholder = "Type your message here..."
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.ShowLineNumbers = true

	replyInput := textinput.New()
	replyInput.Placeholder = "Type your reply here..."
	replyInput.CharLimit = 200
	replyInput.Width = 64

	return model{
		db:             db,
		userKey:        userKey,
		renderer:       renderer,
		currentTheme:   themeGruvbox, // Default theme
		currentScreen:  mainMenu,
		recipientInput: ti,
		messageInput:   &ta,
		rateLimiter:    rateLimiter,
		replyInput:     replyInput,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.loadMessageCount(),
		tickEveryMinute(),
	)
}

// groupMessagesIntoThreads groups messages by sender and returns threads sorted by latest message
func groupMessagesIntoThreads(messages []Message) []messageThread {
	threadMap := make(map[string]*messageThread)

	for _, msg := range messages {
		if thread, exists := threadMap[msg.FromKey]; exists {
			thread.messageCount++
			thread.allMessages = append(thread.allMessages, msg)
			// Update latest message if this one is newer
			if msg.Timestamp.After(thread.latestMessage.Timestamp) {
				thread.latestMessage = msg
			}
		} else {
			threadMap[msg.FromKey] = &messageThread{
				senderKey:     msg.FromKey,
				messageCount:  1,
				latestMessage: msg,
				allMessages:   []Message{msg},
			}
		}
	}

	// Convert map to slice
	threads := make([]messageThread, 0, len(threadMap))
	for _, thread := range threadMap {
		threads = append(threads, *thread)
	}

	// Sort threads by latest message timestamp (newest first)
	// Simple bubble sort for small data sets
	for i := 0; i < len(threads)-1; i++ {
		for j := 0; j < len(threads)-i-1; j++ {
			if threads[j].latestMessage.Timestamp.Before(threads[j+1].latestMessage.Timestamp) {
				threads[j], threads[j+1] = threads[j+1], threads[j]
			}
		}
	}

	return threads
}

func (m model) loadMessageCount() tea.Cmd {
	return func() tea.Msg {
		messages, err := m.db.GetMessagesForUser(m.userKey)
		if err != nil {
			return errMsg{err}
		}
		return len(messages)
	}
}

func tickEveryMinute() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Helper method to get styles from renderer
func (m model) getStyles() styles {
	r := m.renderer
	t := themes[m.currentTheme]

	return styles{
		primaryColor:   t.primary,
		secondaryColor: t.secondary,
		accentColor:    t.accent,
		successColor:   t.success,
		errorColor:     t.error,
		textColor:      t.foreground,
		mutedColor:     t.muted,
		bgColor:        t.background,
		highlight:      t.highlight,
		selectionColor: t.selection,

		titleStyle: r.NewStyle().
			Bold(true).
			Foreground(t.primary).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(t.primary).
			Align(lipgloss.Center),

		subtitleStyle: r.NewStyle().
			Foreground(t.accent).
			Italic(true).
			MarginBottom(1),

		userIDStyle: r.NewStyle().
			Foreground(t.background).
			Background(t.primary).
			Padding(0, 1).
			MarginBottom(1).
			Bold(true),

		menuItemStyle: r.NewStyle().
			Foreground(t.foreground).
			PaddingLeft(2).
			MarginBottom(1),

		menuIconStyle: r.NewStyle().
			Foreground(t.primary).
			Bold(true),

		successStyle: r.NewStyle().
			Foreground(t.success).
			Background(t.background).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.success),

		errorStyle: r.NewStyle().
			Foreground(t.error).
			Background(t.background).
			Padding(0, 2).
			MarginBottom(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.error),

		messageBoxStyle: r.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(t.primary).
			Padding(1, 2).
			MarginBottom(1).
			Width(70),

		messageHeaderStyle: r.NewStyle().
			Foreground(t.secondary).
			Bold(true).
			MarginBottom(1),

		messageTimeStyle: r.NewStyle().
			Foreground(t.muted).
			Italic(true),

		messageContentStyle: r.NewStyle().
			Foreground(t.foreground).
			MarginTop(1),

		newBadgeStyle: r.NewStyle().
			Foreground(t.background).
			Background(t.highlight).
			Padding(0, 1).
			Bold(true),

		inputLabelStyle: r.NewStyle().
			Foreground(t.accent).
			Bold(true).
			MarginBottom(1),

		inputBoxStyle: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.primary).
			Padding(1, 2).
			MarginBottom(1),

		helpStyle: r.NewStyle().
			Foreground(t.muted).
			MarginTop(2).
			Italic(true),

		dividerStyle: r.NewStyle().
			Foreground(t.primary).
			MarginTop(1).
			MarginBottom(1),

		emptyStateStyle: r.NewStyle().
			Foreground(t.muted).
			Italic(true).
			Align(lipgloss.Center).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.muted).
			Padding(2, 4),
	}
}

type styles struct {
	primaryColor        lipgloss.Color
	secondaryColor      lipgloss.Color
	accentColor         lipgloss.Color
	successColor        lipgloss.Color
	errorColor          lipgloss.Color
	textColor           lipgloss.Color
	mutedColor          lipgloss.Color
	bgColor             lipgloss.Color
	highlight           lipgloss.Color
	selectionColor      lipgloss.Color
	titleStyle          lipgloss.Style
	subtitleStyle       lipgloss.Style
	userIDStyle         lipgloss.Style
	menuItemStyle       lipgloss.Style
	menuIconStyle       lipgloss.Style
	successStyle        lipgloss.Style
	errorStyle          lipgloss.Style
	messageBoxStyle     lipgloss.Style
	messageHeaderStyle  lipgloss.Style
	messageTimeStyle    lipgloss.Style
	messageContentStyle lipgloss.Style
	newBadgeStyle       lipgloss.Style
	inputLabelStyle     lipgloss.Style
	inputBoxStyle       lipgloss.Style
	helpStyle           lipgloss.Style
	dividerStyle        lipgloss.Style
	emptyStateStyle     lipgloss.Style
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.currentScreen {
		case mainMenu:
			return m.updateMainMenu(msg)
		case viewMessages:
			return m.updateViewMessages(msg)
		case sendMessageRecipient:
			return m.updateSendMessageRecipient(msg)
		case sendMessageContent:
			return m.updateSendMessageContent(msg)
		}

	case errMsg:
		m.err = msg.err
		return m, nil

	case int:
		// Message count update
		m.messageCount = msg
		return m, nil

	case tickMsg:
		// Periodic message count refresh
		return m, tea.Batch(m.loadMessageCount(), tickEveryMinute())
	}

	// Always update textarea for cursor blink and other internal messages
	if m.currentScreen == sendMessageContent {
		updated, cmd := m.messageInput.Update(msg)
		m.messageInput = &updated
		return m, cmd
	}

	return m, nil
}

func (m model) updateMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "c":
		// Copy SSH key fingerprint to clipboard using OSC 52
		m.successMsg = "SSH key fingerprint copied to clipboard!"
		m.clipboardText = m.userKey
		return m, nil

	// Navigation
	case "j", "down":
		m.selectedMenuItem = (m.selectedMenuItem + 1) % 4
	case "k", "up":
		m.selectedMenuItem = (m.selectedMenuItem - 1 + 4) % 4

	// Selection
	case "enter", " ":
		return m.executeMenuAction()

	// Legacy number keys
	case "1":
		m.selectedMenuItem = 0
		return m.executeMenuAction()
	case "2":
		m.selectedMenuItem = 1
		return m.executeMenuAction()
	case "3":
		m.selectedMenuItem = 2
		return m.executeMenuAction()
	}
	return m, nil
}

func (m model) executeMenuAction() (tea.Model, tea.Cmd) {
	switch m.selectedMenuItem {
	case 0: // View messages
		messages, err := m.db.GetMessagesForUser(m.userKey)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.messages = messages

		// Mark all unread messages as read
		for _, msg := range messages {
			if !msg.Read {
				if err := m.db.MarkMessageAsRead(msg.ID); err != nil {
					m.err = err
					return m, nil
				}
			}
		}

		// Group messages into threads
		m.threads = groupMessagesIntoThreads(messages)
		m.selectedThreadIndex = 0
		m.currentViewMode = viewModeThreaded

		m.currentScreen = viewMessages
		m.selectedMessageIndex = 0
		m.err = nil
		m.successMsg = ""

	case 1: // Send message
		m.currentScreen = sendMessageRecipient
		m.recipientInput.SetValue("")
		m.recipientInput.Focus()
		m.err = nil
		m.successMsg = ""

	case 2: // Change theme
		if m.currentTheme == themeGruvbox {
			m.currentTheme = themeDracula
		} else {
			m.currentTheme = themeGruvbox
		}
		m.err = nil
		m.successMsg = ""

	case 3: // Quit
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateViewMessages(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle reply mode separately
	if m.replyMode {
		switch msg.String() {
		case "enter":
			// Send reply
			var recipientKey string
			if m.currentViewMode == viewModeThreaded && len(m.threads) > 0 {
				recipientKey = m.threads[m.selectedThreadIndex].senderKey
			} else if m.currentViewMode == viewModeConversation {
				recipientKey = m.conversationPartner
			}

			if recipientKey != "" && m.replyInput.Value() != "" {
				replyText := m.replyInput.Value()

				// Check rate limit
				if !m.rateLimiter.CanSendMessage(m.userKey) {
					m.err = fmt.Errorf("rate limit: please wait 10 seconds between messages")
					return m, nil
				}

				// Send reply
				if err := m.db.SendMessage(m.userKey, recipientKey, replyText); err != nil {
					m.err = err
				} else {
					m.rateLimiter.RecordMessage(m.userKey)
					m.successMsg = "Reply sent!"
					m.replyMode = false
					m.replyInput.SetValue("")
					m.replyInput.Blur()
				}
			}
			return m, nil

		case "esc":
			// Cancel reply
			m.replyMode = false
			m.replyInput.SetValue("")
			m.replyInput.Blur()
			return m, nil

		default:
			// Forward to reply input
			var cmd tea.Cmd
			m.replyInput, cmd = m.replyInput.Update(msg)
			return m, cmd
		}
	}

	// Normal mode (not replying)
	switch msg.String() {
	case "q":
		// If in conversation view, go back to threaded view
		if m.currentViewMode == viewModeConversation {
			m.currentViewMode = viewModeThreaded
			m.conversationMessages = nil
			m.conversationPartner = ""
			m.selectedMessageIndex = 0
			m.messageScrollOffset = 0
			m.err = nil
			m.successMsg = ""
		} else {
			// Otherwise go back to main menu
			m.currentScreen = mainMenu
			m.messages = nil
			m.threads = nil
			m.selectedMessageIndex = 0
			m.selectedThreadIndex = 0
			m.messageScrollOffset = 0
			m.replyMode = false
			m.currentViewMode = viewModeThreaded
		}

	case "esc":
		// If in conversation view, go back to threaded view
		if m.currentViewMode == viewModeConversation {
			m.currentViewMode = viewModeThreaded
			m.conversationMessages = nil
			m.conversationPartner = ""
			m.selectedMessageIndex = 0
			m.messageScrollOffset = 0
			m.err = nil
			m.successMsg = ""
		} else {
			// Otherwise go back to main menu
			m.currentScreen = mainMenu
			m.messages = nil
			m.threads = nil
			m.selectedMessageIndex = 0
			m.selectedThreadIndex = 0
			m.messageScrollOffset = 0
			m.replyMode = false
		}

	case "t", "enter":
		// Enter conversation view (show all messages with selected sender)
		if m.currentViewMode == viewModeThreaded && len(m.threads) > 0 {
			thread := m.threads[m.selectedThreadIndex]
			conversationMessages, err := m.db.GetConversationMessages(m.userKey, thread.senderKey)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.conversationMessages = conversationMessages
			m.conversationPartner = thread.senderKey
			m.currentViewMode = viewModeConversation
			m.selectedMessageIndex = 0
			m.messageScrollOffset = 0
			m.err = nil
			m.successMsg = ""
		}

	case "r":
		// Enter reply mode
		if m.currentViewMode == viewModeThreaded && len(m.threads) > 0 {
			m.replyMode = true
			m.replyInput.Focus()
			m.err = nil
			m.successMsg = ""
		} else if m.currentViewMode == viewModeConversation && len(m.conversationMessages) > 0 {
			m.replyMode = true
			m.replyInput.Focus()
			m.err = nil
			m.successMsg = ""
		}

	case "j", "down":
		if m.currentViewMode == viewModeThreaded {
			// Navigate threads
			if len(m.threads) > 0 {
				// Check if current message can scroll down
				currentMsg := m.threads[m.selectedThreadIndex].latestMessage
				msgLines := strings.Split(currentMsg.Message, "\n")
				const maxVisibleLines = 5

				if len(msgLines) > maxVisibleLines {
					maxOffset := len(msgLines) - maxVisibleLines
					if m.messageScrollOffset < maxOffset {
						m.messageScrollOffset++
						return m, nil
					}
				}

				m.selectedThreadIndex = (m.selectedThreadIndex + 1) % len(m.threads)
				m.messageScrollOffset = 0
			}
		} else if m.currentViewMode == viewModeConversation {
			// Navigate conversation messages
			if len(m.conversationMessages) > 0 {
				currentMsg := m.conversationMessages[m.selectedMessageIndex]
				msgLines := strings.Split(currentMsg.Message, "\n")
				const maxVisibleLines = 5

				if len(msgLines) > maxVisibleLines {
					maxOffset := len(msgLines) - maxVisibleLines
					if m.messageScrollOffset < maxOffset {
						m.messageScrollOffset++
						return m, nil
					}
				}

				m.selectedMessageIndex = (m.selectedMessageIndex + 1) % len(m.conversationMessages)
				m.messageScrollOffset = 0
			}
		}

	case "k", "up":
		if m.currentViewMode == viewModeThreaded {
			// Navigate threads
			if len(m.threads) > 0 {
				if m.messageScrollOffset > 0 {
					m.messageScrollOffset--
					return m, nil
				}

				m.selectedThreadIndex = (m.selectedThreadIndex - 1 + len(m.threads)) % len(m.threads)

				newMsg := m.threads[m.selectedThreadIndex].latestMessage
				msgLines := strings.Split(newMsg.Message, "\n")
				const maxVisibleLines = 5
				if len(msgLines) > maxVisibleLines {
					m.messageScrollOffset = len(msgLines) - maxVisibleLines
				} else {
					m.messageScrollOffset = 0
				}
			}
		} else if m.currentViewMode == viewModeConversation {
			// Navigate conversation messages
			if len(m.conversationMessages) > 0 {
				if m.messageScrollOffset > 0 {
					m.messageScrollOffset--
					return m, nil
				}

				m.selectedMessageIndex = (m.selectedMessageIndex - 1 + len(m.conversationMessages)) % len(m.conversationMessages)

				newMsg := m.conversationMessages[m.selectedMessageIndex]
				msgLines := strings.Split(newMsg.Message, "\n")
				const maxVisibleLines = 5
				if len(msgLines) > maxVisibleLines {
					m.messageScrollOffset = len(msgLines) - maxVisibleLines
				} else {
					m.messageScrollOffset = 0
				}
			}
		}

	case "d":
		if m.currentViewMode == viewModeConversation && len(m.conversationMessages) > 0 && m.selectedMessageIndex < len(m.conversationMessages) {
			msgToDelete := m.conversationMessages[m.selectedMessageIndex]
			if err := m.db.DeleteMessage(msgToDelete.ID); err != nil {
				m.err = err
			} else {
				m.successMsg = "Message deleted"
				m.conversationMessages = append(m.conversationMessages[:m.selectedMessageIndex], m.conversationMessages[m.selectedMessageIndex+1:]...)
				if m.selectedMessageIndex >= len(m.conversationMessages) && len(m.conversationMessages) > 0 {
					m.selectedMessageIndex = len(m.conversationMessages) - 1
				}
				m.messageScrollOffset = 0
			}
		}
	}
	return m, nil
}

func (m model) updateSendMessageRecipient(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "enter":
		m.recipient = m.recipientInput.Value()
		if m.recipient == "" {
			m.err = fmt.Errorf("recipient cannot be empty")
			return m, nil
		}
		m.currentScreen = sendMessageContent
		m.recipientInput.Blur()
		m.messageInput.SetValue("")
		m.err = nil
		cmd := m.messageInput.Focus()
		return m, cmd
	case "esc":
		m.currentScreen = mainMenu
		return m, nil
	}

	m.recipientInput, cmd = m.recipientInput.Update(msg)
	return m, cmd
}

func (m model) updateSendMessageContent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+s":
		message := m.messageInput.Value()
		if message == "" {
			m.err = fmt.Errorf("message cannot be empty")
			return m, nil
		}

		// Check rate limit
		if !m.rateLimiter.CanSendMessage(m.userKey) {
			m.err = fmt.Errorf("rate limit: please wait 10 seconds between messages")
			return m, nil
		}

		err := m.db.SendMessage(m.userKey, m.recipient, message)
		if err != nil {
			m.err = err
			return m, nil
		}

		// Record that message was sent
		m.rateLimiter.RecordMessage(m.userKey)

		m.successMsg = "Message sent successfully!"
		m.currentScreen = mainMenu
		m.recipient = ""
		m.err = nil
		return m, nil
	case "esc":
		m.currentScreen = mainMenu
		m.recipient = ""
		return m, nil
	}

	updated, cmd := m.messageInput.Update(msg)
	m.messageInput = &updated
	return m, cmd
}

func (m model) View() string {
	// Handle clipboard copy via OSC 52 if needed
	var clipboardSeq string
	if m.clipboardText != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(m.clipboardText))
		clipboardSeq = fmt.Sprintf("\033]52;c;%s\033\\", encoded)
		m.clipboardText = "" // Clear after using
	}

	var view string
	switch m.currentScreen {
	case mainMenu:
		view = m.viewMainMenu()
	case viewMessages:
		view = m.viewMessagesScreen()
	case sendMessageRecipient:
		view = m.viewSendMessageRecipient()
	case sendMessageContent:
		view = m.viewSendMessageContent()
	}

	// Prepend clipboard sequence if present
	if clipboardSeq != "" {
		return clipboardSeq + view
	}
	return view
}

func (m model) viewMainMenu() string {
	st := m.getStyles()
	var s strings.Builder

	// ASCII Art Title
	asciiArt := []string{
		" █████████            █████████  █████   █████  ███            ████ ",
		" ███░░░░░███          ███░░░░░███░░███   ░░███  ░░░            ░░███ ",
		"░███    ░░░   ██████ ░███    ░░░  ░███    ░███  ████   ██████   ░███ ",
		"░░█████████  ███░░███░░█████████  ░███████████ ░░███  ░░░░░███  ░███ ",
		" ░░░░░░░░███░███ ░███ ░░░░░░░░███ ░███░░░░░███  ░███   ███████  ░███ ",
		" ███    ░███░███ ░███ ███    ░███ ░███    ░███  ░███  ███░░███  ░███ ",
		"░░█████████ ░░██████ ░░█████████  █████   █████ █████░░████████ █████",
		" ░░░░░░░░░   ░░░░░░   ░░░░░░░░░  ░░░░░   ░░░░░ ░░░░░  ░░░░░░░░ ░░░░░ ",
	}

	// Colorize the ASCII art
	// "So" part, "SSH" part (middle), "ial" part (end)
	for _, line := range asciiArt {
		runes := []rune(line)
		if len(runes) > 50 {
			// Split into three parts: So, SSH, ial
			// Positions based on visual letter boundaries in the ASCII art
			part1 := string(runes[0:11])  // S
			part2 := string(runes[12:20]) // o
			part3 := string(runes[21:46]) // SH
			part4 := string(runes[46:])   // ial

			coloredLine := m.renderer.NewStyle().Foreground(st.accentColor).Render(part1) +
				m.renderer.NewStyle().Foreground(st.primaryColor).Render(part2) +
				m.renderer.NewStyle().Foreground(st.accentColor).Render(part3) +
				m.renderer.NewStyle().Foreground(st.primaryColor).Render(part4)
			s.WriteString(coloredLine)
		} else {
			// For shorter lines, just use primary color
			s.WriteString(m.renderer.NewStyle().Foreground(st.primaryColor).Render(line))
		}
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// Centered divider (69 char width, moved 1 char left)
	dividerLine := strings.Repeat("─", 51)
	centeredDivider := m.renderer.NewStyle().
		Foreground(st.secondaryColor).
		Width(69).
		Align(lipgloss.Center).
		Render(dividerLine)
	s.WriteString(centeredDivider)
	s.WriteString("\n")

	// User SSH key - centered
	sshKeyLabel := m.renderer.NewStyle().
		Foreground(st.accentColor).
		Width(69).
		Align(lipgloss.Center).
		Render("Your SSH key fingerprint:")
	s.WriteString(sshKeyLabel)
	s.WriteString("\n")

	// SSH key badge - centered
	sshKeyBadge := m.renderer.NewStyle().
		Foreground(st.textColor).
		Padding(0, 1).
		Bold(true).
		Render(m.userKey)
	centeredBadge := m.renderer.NewStyle().
		Width(69).
		Align(lipgloss.Center).
		Render(sshKeyBadge)
	s.WriteString(centeredBadge)
	s.WriteString("\n")

	// Centered divider
	s.WriteString(centeredDivider)
	s.WriteString("\n\n")

	// Menu options
	viewMessagesText := fmt.Sprintf("✉  View messages (%d)", m.messageCount)
	menuItems := []string{
		viewMessagesText,
		"📝 Send a message",
		"🎨 Change theme",
		"🚪 Quit",
	}

	// Calculate left padding to center the first row (with indicator/spaces)
	// First item with spaces: "  ✉️  View messages" is roughly 21 chars
	// To center in width 69: (69 - 21) / 2 ≈ 24 spaces
	leftPadding := 22

	for i, item := range menuItems {
		// Split emoji from text (emoji is first rune + space)
		runes := []rune(item)
		emoji := string(runes[0:2]) // emoji + space
		text := string(runes[2:])   // rest of the text

		// Build the full menu line with consistent left padding
		var menuLine string
		if i == m.selectedMenuItem {
			// Selected: use selection color for both emoji and text
			indicator := m.renderer.NewStyle().Foreground(st.accentColor).Render("▶ ")
			styledEmoji := m.renderer.NewStyle().Foreground(st.selectionColor).Bold(true).Render(emoji)
			styledText := m.renderer.NewStyle().Foreground(st.selectionColor).Bold(true).Render(text)
			menuLine = strings.Repeat(" ", leftPadding) + indicator + styledEmoji + styledText
		} else {
			// Unselected: pink/purple for emoji, normal text color for text
			styledEmoji := m.renderer.NewStyle().Foreground(st.secondaryColor).Render(emoji)
			styledText := m.renderer.NewStyle().Foreground(st.textColor).Render(text)
			menuLine = strings.Repeat(" ", leftPadding) + "  " + styledEmoji + styledText
		}

		s.WriteString(menuLine)
		s.WriteString("\n")
	}

	// Success or error messages (fixed height to keep bottom elements stable)
	s.WriteString("\n")
	if m.successMsg != "" {
		// Special handling for theme change messages
		if strings.Contains(m.successMsg, "Theme changed to") {
			var themeName string
			var themeColors []lipgloss.Color

			if strings.Contains(m.successMsg, "Gruvbox") {
				themeName = "Gruvbox"
				// Gruvbox color palette for each letter (official bright colors)
				themeColors = []lipgloss.Color{
					lipgloss.Color("#FABD2F"), // G - yellow (primary)
					lipgloss.Color("#8EC07C"), // r - aqua (accent)
					lipgloss.Color("#D3869B"), // u - purple (secondary)
					lipgloss.Color("#B8BB26"), // v - green (success)
					lipgloss.Color("#FE8019"), // b - orange (highlight)
					lipgloss.Color("#FB4934"), // o - red (error)
					lipgloss.Color("#83A598"), // x - blue (bright)
				}
			} else if strings.Contains(m.successMsg, "Dracula") {
				themeName = "Dracula"
				// Dracula color palette for each letter
				themeColors = []lipgloss.Color{
					lipgloss.Color("#bd93f9"), // D - purple (primary)
					lipgloss.Color("#8be9fd"), // r - cyan (accent)
					lipgloss.Color("#ff79c6"), // a - pink (secondary)
					lipgloss.Color("#50fa7b"), // c - green (success)
					lipgloss.Color("#ffb86c"), // u - orange (highlight)
					lipgloss.Color("#f1fa8c"), // l - yellow
					lipgloss.Color("#ff5555"), // a - red (error)
				}
			}

			// Build the colorized theme name
			var coloredThemeName strings.Builder
			for i, char := range themeName {
				coloredChar := m.renderer.NewStyle().
					Foreground(themeColors[i]).
					Bold(true).
					Render(string(char))
				coloredThemeName.WriteString(coloredChar)
			}

			checkmark := m.renderer.NewStyle().Foreground(st.successColor).Render("✓")
			normalText := m.renderer.NewStyle().Foreground(st.textColor).Render(" Theme changed to ")

			// Center the entire message
			fullMessage := checkmark + normalText + coloredThemeName.String()
			centeredMessage := m.renderer.NewStyle().
				Width(69).
				Align(lipgloss.Center).
				Render(fullMessage)

			s.WriteString(centeredMessage)
		} else {
			// Other success messages without background
			s.WriteString(m.renderer.NewStyle().Foreground(st.successColor).Render("  ✓ " + m.successMsg))
		}
	} else if m.err != nil {
		s.WriteString(m.renderer.NewStyle().Foreground(st.errorColor).Render("  ✗ " + m.err.Error()))
	}
	s.WriteString("\n")

	// Help text
	s.WriteString(st.helpStyle.Render("j/k or ↑/↓ to navigate • Enter to select • c to copy key • q to quit"))

	// Theme indicator in bottom left
	themeText := fmt.Sprintf("\n\nTheme: %s", themes[m.currentTheme].name)
	s.WriteString(m.renderer.NewStyle().Foreground(st.mutedColor).Render(themeText))

	return s.String()
}

func (m model) viewMessagesScreen() string {
	if m.currentViewMode == viewModeThreaded {
		return m.viewThreadedMessages()
	}
	return m.viewConversationMessages()
}

func (m model) viewThreadedMessages() string {
	st := m.getStyles()
	var s strings.Builder

	// Title
	title := st.titleStyle.Width(70).Render("✉  Your Messages")
	s.WriteString(title)
	s.WriteString("\n")

	// Fixed height container for messages (always same height)
	var messagesContent strings.Builder

	if len(m.threads) == 0 {
		// Empty state
		emptyMsg := st.emptyStateStyle.Width(70).Render("📭 No messages yet!\n\nYour inbox is empty.")
		messagesContent.WriteString(emptyMsg)
	} else {
		// Show max 3 threads at a time (1 expanded, 2 compact)
		const maxVisible = 3

		// Calculate which threads to show
		var startIdx, endIdx int
		if m.selectedThreadIndex == 0 {
			startIdx = 0
			endIdx = maxVisible
			if endIdx > len(m.threads) {
				endIdx = len(m.threads)
			}
		} else if m.selectedThreadIndex >= len(m.threads)-1 {
			endIdx = len(m.threads)
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		} else {
			startIdx = m.selectedThreadIndex - 1
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + maxVisible
			if endIdx > len(m.threads) {
				endIdx = len(m.threads)
			}
		}

		// Display the visible threads
		for i := startIdx; i < endIdx; i++ {
			thread := m.threads[i]
			msg := thread.latestMessage

			if i == m.selectedThreadIndex {
				// Selected thread - full expanded view with fixed content height
				var messageContent strings.Builder

				// Header with sender and thread count
				header := st.messageHeaderStyle.Render(fmt.Sprintf("From: %s", msg.FromKey))
				if !msg.Read {
					header += " " + st.newBadgeStyle.Render(" NEW ")
				}
				// Add thread indicator if there are multiple messages
				if thread.messageCount > 1 {
					threadBadge := m.renderer.NewStyle().
						Foreground(st.accentColor).
						Render(fmt.Sprintf(" 🧵 %d", thread.messageCount))
					header += " " + threadBadge
				}
				messageContent.WriteString(header)
				messageContent.WriteString("\n")

				// Timestamp and helper text on same line
				msgLines := strings.Split(msg.Message, "\n")

				// Show 4 lines when replying to make room for reply input, otherwise 5
				maxMessageLines := 5
				if m.replyMode {
					maxMessageLines = 4
				}

				timeStr := st.messageTimeStyle.Render(msg.Timestamp.Format("Mon, Jan 2 2006 at 15:04"))

				// Add helper text on same line if message is longer than max lines
				if len(msgLines) > maxMessageLines {
					helperStyle := m.renderer.NewStyle().Foreground(st.mutedColor)
					helperText := helperStyle.Render("j/k ↑↓ to view full message")

					// Width is 70, padding(1,2) means 66 internal width
					const internalWidth = 66
					leftWidth := lipgloss.Width(timeStr)
					rightWidth := lipgloss.Width(helperText)
					spacingWidth := internalWidth - leftWidth - rightWidth
					if spacingWidth < 1 {
						spacingWidth = 1
					}

					timestampLine := timeStr + strings.Repeat(" ", spacingWidth) + helperText
					messageContent.WriteString(timestampLine)
				} else {
					messageContent.WriteString(timeStr)
				}
				messageContent.WriteString("\n")

				// Message content - show 4 or 5 lines depending on reply mode

				// Display message lines with scroll offset
				startLine := m.messageScrollOffset
				endLine := startLine + maxMessageLines
				if endLine > len(msgLines) {
					endLine = len(msgLines)
				}

				for j := 0; j < maxMessageLines; j++ {
					lineIdx := startLine + j
					if lineIdx < len(msgLines) {
						messageContent.WriteString(msgLines[lineIdx])
					}
					if j < maxMessageLines-1 {
						messageContent.WriteString("\n")
					}
				}

				// Add reply input if in reply mode (replaces one line of message content)
				if m.replyMode {
					messageContent.WriteString("\n")

					// Create full-width reply row with background
					const internalWidth = 66

					// Get input view
					inputView := m.replyInput.View()

					// Calculate padding needed to fill width
					currentWidth := lipgloss.Width(inputView)
					paddingNeeded := internalWidth - currentWidth
					if paddingNeeded < 0 {
						paddingNeeded = 0
					}

					// Create the full row with background
					fullRow := inputView + strings.Repeat(" ", paddingNeeded)

					// Apply background to the entire row
					rowStyle := m.renderer.NewStyle().
						Background(st.mutedColor).
						Width(internalWidth)

					styledRow := rowStyle.Render(fullRow)
					messageContent.WriteString(styledRow)
				}

				// Full box with highlighted border
				selectedStyle := m.renderer.NewStyle().
					Border(lipgloss.ThickBorder()).
					BorderForeground(st.accentColor).
					Padding(1, 2).
					MarginBottom(1).
					Width(70)
				box := selectedStyle.Render(messageContent.String())
				messagesContent.WriteString(box)
			} else {
				// Unselected thread - compact one-line view
				// Truncate sender to max 20 chars
				sender := msg.FromKey
				if len(sender) > 20 {
					sender = sender[:17] + "..."
				}

				// Format date and time as YYYY-MM-DD HH:MM
				dateTimeStr := msg.Timestamp.Format("2006-01-02 15:04")

				// Calculate direction indicator
				var directionText string
				isMiddleBox := (m.selectedThreadIndex == 0 && i == startIdx+1) ||
					(m.selectedThreadIndex == len(m.threads)-1 && i == endIdx-2)

				if !isMiddleBox {
					if i < m.selectedThreadIndex {
						if i == 0 {
							directionText = "  Newest"
						} else {
							newerCount := i
							directionText = fmt.Sprintf("  %d newer...", newerCount)
						}
					} else if i > m.selectedThreadIndex {
						if i == len(m.threads)-1 {
							directionText = "  Oldest"
						} else {
							olderCount := len(m.threads) - i - 1
							directionText = fmt.Sprintf("  %d older...", olderCount)
						}
					}
				}

				// Build left part (sender) and right part (timestamp + direction)
				leftPart := fmt.Sprintf("From: %s", sender)
				if !msg.Read {
					leftPart += " " + st.newBadgeStyle.Render(" NEW ")
				}
				// Add compact thread count indicator
				if thread.messageCount > 1 {
					leftPart += fmt.Sprintf(" (🧵 %d)", thread.messageCount)
				}

				rightPart := dateTimeStr + directionText

				// Width is 70, padding(0,1) means 68 internal width
				// Calculate spacing to push right part to the right
				const internalWidth = 68
				leftWidth := lipgloss.Width(leftPart)
				rightWidth := lipgloss.Width(rightPart)
				spacingWidth := internalWidth - leftWidth - rightWidth
				if spacingWidth < 1 {
					spacingWidth = 1
				}

				compactLine := leftPart + strings.Repeat(" ", spacingWidth) + rightPart

				// Compact box
				compactStyle := m.renderer.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(st.mutedColor).
					Padding(0, 1).
					MarginBottom(1).
					Width(70)
				box := compactStyle.Render(compactLine)
				messagesContent.WriteString(box)
			}

			if i < endIdx-1 {
				messagesContent.WriteString("\n")
			}
		}
	}

	// Manually pad to fixed height
	content := messagesContent.String()

	// Count actual rendered lines by splitting on newlines
	renderedLines := strings.Split(content, "\n")
	actualLineCount := len(renderedLines)

	// Target: 1 expanded box (~9 lines) + 2 compact boxes (~3 lines each) = ~15 lines
	targetLines := 15

	s.WriteString(content)

	// Add padding newlines to reach exact target height
	paddingNeeded := targetLines - actualLineCount
	if paddingNeeded > 0 {
		s.WriteString(strings.Repeat("\n", paddingNeeded))
	}

	// Success or error messages (fixed height to keep bottom elements stable)
	if m.successMsg != "" {
		s.WriteString(m.renderer.NewStyle().Foreground(st.successColor).Render("  ✓ " + m.successMsg))
	} else if m.err != nil {
		s.WriteString(m.renderer.NewStyle().Foreground(st.errorColor).Render("  ✗ " + m.err.Error()))
	}

	// Help text depends on reply mode
	var helpText string
	if m.replyMode {
		helpText = "enter to send reply • esc to cancel"
	} else {
		helpText = "j/k or ↑/↓ to navigate • enter/t to view thread • r to reply • q/esc to return"
	}
	s.WriteString(st.helpStyle.Render(helpText))

	return s.String()
}

func (m model) viewConversationMessages() string {
	st := m.getStyles()
	var s strings.Builder

	// Title with conversation partner
	title := st.titleStyle.Width(70).Render(fmt.Sprintf("💬  Conversation with %s", m.conversationPartner))
	s.WriteString(title)
	s.WriteString("\n")

	// Fixed height container for messages
	var messagesContent strings.Builder

	if len(m.conversationMessages) == 0 {
		// Empty state
		emptyMsg := st.emptyStateStyle.Width(70).Render("📭 No messages in this conversation!")
		messagesContent.WriteString(emptyMsg)
	} else {
		// Show max 3 messages at a time (1 expanded, 2 compact)
		const maxVisible = 3

		// Calculate which messages to show
		var startIdx, endIdx int
		if m.selectedMessageIndex == 0 {
			startIdx = 0
			endIdx = maxVisible
			if endIdx > len(m.conversationMessages) {
				endIdx = len(m.conversationMessages)
			}
		} else if m.selectedMessageIndex >= len(m.conversationMessages)-1 {
			endIdx = len(m.conversationMessages)
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		} else {
			startIdx = m.selectedMessageIndex - 1
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + maxVisible
			if endIdx > len(m.conversationMessages) {
				endIdx = len(m.conversationMessages)
			}
		}

		// Display the visible messages
		for i := startIdx; i < endIdx; i++ {
			msg := m.conversationMessages[i]
			isFromUser := msg.FromKey == m.userKey

			if i == m.selectedMessageIndex {
				// Selected message - full expanded view
				var messageContent strings.Builder

				// Header with sender indicator
				var header string
				if isFromUser {
					header = st.messageHeaderStyle.Render("You")
				} else {
					header = st.messageHeaderStyle.Render(fmt.Sprintf("From: %s", msg.FromKey))
				}
				if !msg.Read && !isFromUser {
					header += " " + st.newBadgeStyle.Render(" NEW ")
				}
				messageContent.WriteString(header)
				messageContent.WriteString("\n")

				// Timestamp
				msgLines := strings.Split(msg.Message, "\n")
				maxMessageLines := 5
				if m.replyMode {
					maxMessageLines = 4
				}

				timeStr := st.messageTimeStyle.Render(msg.Timestamp.Format("Mon, Jan 2 2006 at 15:04"))

				if len(msgLines) > maxMessageLines {
					helperStyle := m.renderer.NewStyle().Foreground(st.mutedColor)
					helperText := helperStyle.Render("j/k ↑↓ to view full message")

					const internalWidth = 66
					leftWidth := lipgloss.Width(timeStr)
					rightWidth := lipgloss.Width(helperText)
					spacingWidth := internalWidth - leftWidth - rightWidth
					if spacingWidth < 1 {
						spacingWidth = 1
					}

					timestampLine := timeStr + strings.Repeat(" ", spacingWidth) + helperText
					messageContent.WriteString(timestampLine)
				} else {
					messageContent.WriteString(timeStr)
				}
				messageContent.WriteString("\n")

				// Message content with scroll
				startLine := m.messageScrollOffset
				endLine := startLine + maxMessageLines
				if endLine > len(msgLines) {
					endLine = len(msgLines)
				}

				for j := 0; j < maxMessageLines; j++ {
					lineIdx := startLine + j
					if lineIdx < len(msgLines) {
						messageContent.WriteString(msgLines[lineIdx])
					}
					if j < maxMessageLines-1 {
						messageContent.WriteString("\n")
					}
				}

				// Add reply input if in reply mode
				if m.replyMode {
					messageContent.WriteString("\n")

					const internalWidth = 66
					inputView := m.replyInput.View()
					currentWidth := lipgloss.Width(inputView)
					paddingNeeded := internalWidth - currentWidth
					if paddingNeeded < 0 {
						paddingNeeded = 0
					}

					fullRow := inputView + strings.Repeat(" ", paddingNeeded)
					rowStyle := m.renderer.NewStyle().
						Background(st.mutedColor).
						Width(internalWidth)

					styledRow := rowStyle.Render(fullRow)
					messageContent.WriteString(styledRow)
				}

				// Choose border color based on sender
				borderColor := st.accentColor
				if isFromUser {
					borderColor = st.secondaryColor
				}

				selectedStyle := m.renderer.NewStyle().
					Border(lipgloss.ThickBorder()).
					BorderForeground(borderColor).
					Padding(1, 2).
					MarginBottom(1).
					Width(70)
				box := selectedStyle.Render(messageContent.String())
				messagesContent.WriteString(box)
			} else {
				// Unselected message - compact one-line view
				var senderStr string
				if isFromUser {
					senderStr = "You"
				} else {
					sender := msg.FromKey
					if len(sender) > 20 {
						sender = sender[:17] + "..."
					}
					senderStr = sender
				}

				dateTimeStr := msg.Timestamp.Format("2006-01-02 15:04")

				// Direction indicator
				var directionText string
				isMiddleBox := (m.selectedMessageIndex == 0 && i == startIdx+1) ||
					(m.selectedMessageIndex == len(m.conversationMessages)-1 && i == endIdx-2)

				if !isMiddleBox {
					if i < m.selectedMessageIndex {
						if i == 0 {
							directionText = "  Newest"
						} else {
							newerCount := i
							directionText = fmt.Sprintf("  %d newer...", newerCount)
						}
					} else if i > m.selectedMessageIndex {
						if i == len(m.conversationMessages)-1 {
							directionText = "  Oldest"
						} else {
							olderCount := len(m.conversationMessages) - i - 1
							directionText = fmt.Sprintf("  %d older...", olderCount)
						}
					}
				}

				// Build the compact line
				var leftPart string
				if isFromUser {
					leftPart = senderStr
				} else {
					leftPart = fmt.Sprintf("From: %s", senderStr)
				}
				if !msg.Read && !isFromUser {
					leftPart += " " + st.newBadgeStyle.Render(" NEW ")
				}

				rightPart := dateTimeStr + directionText

				const internalWidth = 68
				leftWidth := lipgloss.Width(leftPart)
				rightWidth := lipgloss.Width(rightPart)
				spacingWidth := internalWidth - leftWidth - rightWidth
				if spacingWidth < 1 {
					spacingWidth = 1
				}

				compactLine := leftPart + strings.Repeat(" ", spacingWidth) + rightPart

				compactStyle := m.renderer.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(st.mutedColor).
					Padding(0, 1).
					MarginBottom(1).
					Width(70)
				box := compactStyle.Render(compactLine)
				messagesContent.WriteString(box)
			}

			if i < endIdx-1 {
				messagesContent.WriteString("\n")
			}
		}
	}

	// Manually pad to fixed height
	content := messagesContent.String()
	renderedLines := strings.Split(content, "\n")
	actualLineCount := len(renderedLines)
	targetLines := 15

	s.WriteString(content)

	paddingNeeded := targetLines - actualLineCount
	if paddingNeeded > 0 {
		s.WriteString(strings.Repeat("\n", paddingNeeded))
	}

	// Success or error messages
	if m.successMsg != "" {
		s.WriteString(m.renderer.NewStyle().Foreground(st.successColor).Render("  ✓ " + m.successMsg))
	} else if m.err != nil {
		s.WriteString(m.renderer.NewStyle().Foreground(st.errorColor).Render("  ✗ " + m.err.Error()))
	}

	// Help text
	var helpText string
	if m.replyMode {
		helpText = "enter to send reply • esc to cancel"
	} else {
		helpText = "j/k or ↑/↓ to navigate • r to reply • d to delete • esc/q to go back"
	}
	s.WriteString(st.helpStyle.Render(helpText))

	return s.String()
}

func (m model) viewSendMessageRecipient() string {
	st := m.getStyles()
	var s strings.Builder

	// Title
	title := st.titleStyle.Width(70).Render("📝  Send Message")
	s.WriteString(title)
	s.WriteString("\n\n")

	// Instructions
	s.WriteString(st.inputLabelStyle.Render("Step 1 of 2: Enter Recipient"))
	s.WriteString("\n\n")

	// Error message (fixed height to keep bottom elements stable)
	if m.err != nil {
		s.WriteString(st.errorStyle.Render(" ✗ " + m.err.Error() + " "))
	}
	s.WriteString("\n\n")

	// Input box
	input := st.inputBoxStyle.Width(70).Render(m.recipientInput.View())
	s.WriteString(input)
	s.WriteString("\n")

	// Help text
	s.WriteString(st.helpStyle.Render("Press [enter] to continue • [esc] to cancel"))

	return s.String()
}

func (m model) viewSendMessageContent() string {
	st := m.getStyles()
	var s strings.Builder

	// Title
	title := st.titleStyle.Width(70).Render("📝  Send Message")
	s.WriteString(title)
	s.WriteString("\n\n")

	// Instructions
	s.WriteString(st.inputLabelStyle.Render("Step 2 of 2: Write Your Message"))
	s.WriteString("\n")

	// Recipient info
	recipientBox := m.renderer.NewStyle().
		Foreground(st.secondaryColor).
		Render(fmt.Sprintf("To: %s", m.recipient))
	s.WriteString(recipientBox)
	s.WriteString("\n")

	// Error message (fixed height to keep bottom elements stable)
	if m.err != nil {
		s.WriteString(st.errorStyle.Render(" ✗ " + m.err.Error() + " "))
	}
	s.WriteString("\n\n")

	// Message input box
	input := st.inputBoxStyle.Width(70).Render(m.messageInput.View())
	s.WriteString(input)
	s.WriteString("\n")

	// Help text
	s.WriteString(st.helpStyle.Render("Press [ctrl+s] to send • [esc] to cancel"))

	return s.String()
}
