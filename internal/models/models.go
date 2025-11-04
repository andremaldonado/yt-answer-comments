package models

import "time"

type Comment struct {
	CommentText string
	Response    string
	CreatedAt   time.Time
}

type SentimentAnalysis struct {
	Sentimento string `json:"sentimento"`
	Nota       int    `json:"nota"`
	Tema       string `json:"tema"`
}
