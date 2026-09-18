package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"answer-comments/internal/database"
	yt "answer-comments/internal/youtube"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Config struct {
	ClientSecretFile          string
	LLMAPIKey                 string
	MembersCSVFile            string
	CommentsDatabaseURL       string
	TranscriptionsDatabaseURL string
	TokenFile                 string
}

type App struct {
	Config          *Config
	YTService       *youtube.Service
	LLMClient       openai.Client
	ChannelID       string
	TranscriptionDB *sql.DB
}

func NewApp(ctx context.Context, transcriptionMode bool) (*App, error) {
	// Load config.env file
	if err := godotenv.Load("config.env"); err != nil {
		log.Printf("Aviso: Arquivo .env não encontrado. Usando variáveis de ambiente do sistema.")
	}

	appConfig := &Config{
		ClientSecretFile:          getEnv("CLIENT_SECRET_FILE", "data/client_secret.json"),
		LLMAPIKey:                 os.Getenv("LLM_API_KEY"),
		MembersCSVFile:            getEnv("MEMBERS_CSV_FILE", "data/members.csv"),
		CommentsDatabaseURL:       os.Getenv("COMMENTS_DATABASE_URL"),
		TranscriptionsDatabaseURL: os.Getenv("TRANSCRIPTIONS_DATABASE_URL"),
		TokenFile:                 getEnv("TOKEN_FILE", "data/token.json"),
	}

	if appConfig.LLMAPIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY não configurada")
	}
	if appConfig.CommentsDatabaseURL == "" {
		return nil, fmt.Errorf("COMMENTS_DATABASE_URL não configurada")
	}
	if appConfig.TranscriptionsDatabaseURL == "" {
		return nil, fmt.Errorf("TRANSCRIPTIONS_DATABASE_URL não configurada")
	}

	// Initialize comments database (PostgreSQL)
	if err := database.InitDB(appConfig.CommentsDatabaseURL); err != nil {
		return nil, fmt.Errorf("erro ao inicializar o banco de comentários: %w", err)
	}

	// YouTube Client
	b, err := os.ReadFile(appConfig.ClientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler o arquivo %s: %w", appConfig.ClientSecretFile, err)
	}

	scopes := []string{yt.YoutubeForceSslScope, yt.YoutubeChannelMembershipsCreatorScope}
	if transcriptionMode {
		scopes = append(scopes, youtube.YoutubeScope)
	}
	oauthConfig, err := google.ConfigFromJSON(b, scopes...)
	if err != nil {
		return nil, fmt.Errorf("não foi possível analisar o arquivo de segredo do cliente: %w", err)
	}

	client, err := yt.GetYoutubeClient(oauthConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter cliente do YouTube: %w", err)
	}

	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar o serviço do YouTube: %w", err)
	}

	// Get Channel ID
	channelResponse, err := service.Channels.List([]string{"id"}).Mine(true).Do()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter o ID do canal: %w", err)
	}
	if len(channelResponse.Items) == 0 {
		return nil, fmt.Errorf("não foi possível encontrar o ID do canal do usuário autenticado")
	}
	channelID := channelResponse.Items[0].Id

	// LLM Client (DeepSeek)
	llmClient := openai.NewClient(
		openaiopt.WithAPIKey(appConfig.LLMAPIKey),
		openaiopt.WithBaseURL("https://api.deepseek.com/v1"),
	)

	app := &App{
		Config:    appConfig,
		YTService: service,
		LLMClient: llmClient,
		ChannelID: channelID,
	}

	// Transcription DB (PostgreSQL)
	pgDB, err := sql.Open("postgres", appConfig.TranscriptionsDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao banco de transcrições: %w", err)
	}
	if err := pgDB.Ping(); err != nil {
		pgDB.Close()
		return nil, fmt.Errorf("banco de transcrições inacessível: %w", err)
	}
	app.TranscriptionDB = pgDB

	return app, nil
}

func (a *App) Close() {
	database.CloseDB()
	if a.TranscriptionDB != nil {
		a.TranscriptionDB.Close()
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
