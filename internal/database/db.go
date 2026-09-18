package database

import (
	"database/sql"
	"time"

	"answer-comments/internal/models"

	_ "github.com/lib/pq"
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

// InitDB initializes the PostgreSQL database connection and creates tables if needed
func InitDB(databaseURL string) error {
	var err error
	db, err = sql.Open("postgres", databaseURL)
	if err != nil {
		return err
	}

	if err = db.Ping(); err != nil {
		return err
	}

	// Create comments table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			author TEXT NOT NULL,
			comment_text TEXT NOT NULL,
			sentiment TEXT NOT NULL,
			score INTEGER NOT NULL,
			response TEXT,
			theme TEXT,
			user_answered BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL,
			responded_at TIMESTAMPTZ,
			video_id TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE comments ADD COLUMN IF NOT EXISTS theme TEXT`)
	if err != nil {
		return err
	}

	return nil
}

// SaveComment stores a comment and its response in the database
func SaveComment(comment *youtube.Comment, sentiment string, score int, theme string, response string, userAnswered bool) error {
	createdAt, err := time.Parse(time.RFC3339, comment.Snippet.PublishedAt)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO comments (
			id, author, comment_text, sentiment, score, response, theme,
			user_answered, created_at, responded_at, video_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		comment.Id,
		comment.Snippet.AuthorDisplayName,
		comment.Snippet.TextOriginal,
		sentiment,
		score,
		response,
		theme,
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
		SELECT id, author, comment_text, response, created_at
		FROM comments
		WHERE author = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, author, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var id, author string
		var c models.Comment
		err := rows.Scan(&id, &author, &c.CommentText, &c.Response, &c.CreatedAt)
		if err != nil {
			return nil, err
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

// GetPreviousAnswersByContext retrieves previous answers with similar theme and sentiment
func GetPreviousAnswersByContext(theme string, sentiment string, limit int) ([]string, error) {
	rows, err := db.Query(`
		SELECT comment_text, response
		FROM comments
		WHERE theme = $1
		AND sentiment = $2
		AND response != ''
		AND user_answered = true
		ORDER BY responded_at DESC
		LIMIT $3
	`, theme, sentiment, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var comment, response string
		if err := rows.Scan(&comment, &response); err != nil {
			return nil, err
		}
		// Format the result as "Pergunta: {comment}\nResposta: {response}\n"
		contextEntry := "Pergunta: " + comment + "\nResposta: " + response + "\n"
		results = append(results, contextEntry)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
