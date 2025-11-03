package models

import "time"

type Comment struct {
	CommentText string
	Response    string
	CreatedAt   time.Time
}

type SentimentAnalysis struct {
	Sentimento string
	Nota       int
}
