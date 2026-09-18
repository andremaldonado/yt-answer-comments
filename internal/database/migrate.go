package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// MigrateFromSQLite reads all rows from the legacy SQLite comments.db file and
// inserts them into the PostgreSQL comments table, skipping rows that already exist.
func MigrateFromSQLite(sqlitePath string, pgDB *sql.DB) (int, error) {
	sqliteDB, err := sql.Open("sqlite3", sqlitePath)
	if err != nil {
		return 0, fmt.Errorf("erro ao abrir o SQLite: %w", err)
	}
	defer sqliteDB.Close()

	rows, err := sqliteDB.Query(`
		SELECT id, author, comment_text, sentiment, score, response, theme,
			user_answered, created_at, responded_at, video_id
		FROM comments
	`)
	if err != nil {
		return 0, fmt.Errorf("erro ao ler comentários do SQLite: %w", err)
	}
	defer rows.Close()

	migrated := 0
	for rows.Next() {
		var (
			id, author, commentText, sentiment, response, videoID string
			theme                                                 sql.NullString
			score                                                 int
			userAnswered                                          bool
			createdAt                                             string
			respondedAt                                           sql.NullString
		)
		if err := rows.Scan(&id, &author, &commentText, &sentiment, &score, &response,
			&theme, &userAnswered, &createdAt, &respondedAt, &videoID); err != nil {
			return migrated, fmt.Errorf("erro ao escanear linha do SQLite: %w", err)
		}

		result, err := pgDB.Exec(`
			INSERT INTO comments (
				id, author, comment_text, sentiment, score, response, theme,
				user_answered, created_at, responded_at, video_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO NOTHING
		`, id, author, commentText, sentiment, score, response, theme,
			userAnswered, createdAt, respondedAt, videoID)
		if err != nil {
			return migrated, fmt.Errorf("erro ao inserir comentário %s no Postgres: %w", id, err)
		}

		if affected, _ := result.RowsAffected(); affected > 0 {
			migrated++
		}
	}
	if err := rows.Err(); err != nil {
		return migrated, err
	}

	return migrated, nil
}
