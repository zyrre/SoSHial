package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type User struct {
	SSHKeyFingerprint string
	FirstSeen         time.Time
	LastSeen          time.Time
}

type Message struct {
	ID        int64
	FromKey   string
	ToKey     string
	Message   string
	Timestamp time.Time
	Read      bool
}

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	database := &Database{db: db}
	if err := database.initSchema(); err != nil {
		return nil, err
	}

	return database, nil
}

func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		ssh_key_fingerprint TEXT PRIMARY KEY,
		first_seen DATETIME NOT NULL,
		last_seen DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_key TEXT NOT NULL,
		to_key TEXT NOT NULL,
		message TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		read BOOLEAN DEFAULT 0,
		FOREIGN KEY (from_key) REFERENCES users(ssh_key_fingerprint),
		FOREIGN KEY (to_key) REFERENCES users(ssh_key_fingerprint)
	);

	CREATE INDEX IF NOT EXISTS idx_messages_to_key ON messages(to_key);
	CREATE INDEX IF NOT EXISTS idx_messages_from_key ON messages(from_key);
	`

	_, err := d.db.Exec(schema)
	return err
}

func (d *Database) UpsertUser(fingerprint string) error {
	now := time.Now()

	_, err := d.db.Exec(`
		INSERT INTO users (ssh_key_fingerprint, first_seen, last_seen)
		VALUES (?, ?, ?)
		ON CONFLICT(ssh_key_fingerprint) DO UPDATE SET last_seen = ?
	`, fingerprint, now, now, now)

	return err
}

func (d *Database) GetMessagesForUser(fingerprint string) ([]Message, error) {
	rows, err := d.db.Query(`
		SELECT id, from_key, to_key, message, timestamp, read
		FROM messages
		WHERE to_key = ?
		ORDER BY timestamp DESC
	`, fingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.FromKey, &msg.ToKey, &msg.Message, &msg.Timestamp, &msg.Read); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

func (d *Database) SendMessage(fromKey, toKey, message string) error {
	_, err := d.db.Exec(`
		INSERT INTO messages (from_key, to_key, message, timestamp, read)
		VALUES (?, ?, ?, ?, 0)
	`, fromKey, toKey, message, time.Now())

	return err
}

func (d *Database) MarkMessageAsRead(messageID int64) error {
	_, err := d.db.Exec(`
		UPDATE messages SET read = 1 WHERE id = ?
	`, messageID)

	return err
}

func (d *Database) DeleteMessage(messageID int64) error {
	_, err := d.db.Exec(`
		DELETE FROM messages WHERE id = ?
	`, messageID)

	return err
}

func (d *Database) Close() error {
	return d.db.Close()
}
