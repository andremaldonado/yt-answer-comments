package database

import (
	"database/sql"
	"time"

	"answer-comments/internal/models"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/youtube/v3"
)

// DBComment represents a YouTube comment and its response in the database
type DBComment struct {
	ID           string    // YouTube comment ID
	Author       string    // YouTube username
	CommentText  string    // Original comment text
	Sentiment    string    // Sentiment analysis result
	Score        int       // Understanding score (1-5)
	Response     string    // Response text
	UserAnswered bool      // Whether response was edited by user
	CreatedAt    time.Time // When the comment was posted
	RespondedAt  time.Time // When we responded
}

var db *sql.DB

// InitDB initializes the SQLite database connection and creates tables if needed
func InitDB() error {
	var err error
	db, err = sql.Open("sqlite3", "comments.db")
	if err != nil {
		return err
	}

	// Create comments table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			author TEXT NOT NULL,
			comment_text TEXT NOT NULL,
			sentiment TEXT NOT NULL,
			score INTEGER NOT NULL,
			response TEXT,
			user_answered BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			responded_at DATETIME,
			video_id TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	return nil
}

// SaveComment stores a comment and its response in the database
func SaveComment(comment *youtube.Comment, sentiment string, score int, response string, userAnswered bool) error {
	createdAt, err := time.Parse(time.RFC3339, comment.Snippet.PublishedAt)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO comments (
			id, author, comment_text, sentiment, score, response, 
			user_answered, created_at, responded_at, video_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		comment.Id,
		comment.Snippet.AuthorDisplayName,
		comment.Snippet.TextOriginal,
		sentiment,
		score,
		response,
		userAnswered,
		createdAt,
		time.Now(),
		comment.Snippet.VideoId,
	)
	return err
}

// GetLastComments retorna os últimos N comentários e respostas do mesmo autor
func GetLastComments(author string, limit int) ([]models.Comment, error) {
	rows, err := db.Query(`
		SELECT id, author, comment_text, response, datetime(created_at) as created_at
		FROM comments
		WHERE author = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, author, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var id, author string
		var c models.Comment
		var createdAt string // SQLite armazena datetime como string
		err := rows.Scan(&id, &author, &c.CommentText, &c.Response, &createdAt)
		if err != nil {
			return nil, err
		}
		c.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			// Try the old format as fallback
			c.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
			if err != nil {
				return nil, err
			}
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		db.Close()
	}
}
