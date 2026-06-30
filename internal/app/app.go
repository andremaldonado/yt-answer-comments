package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"answer-comments/internal/database"
	"answer-comments/internal/ui"
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
	ClientSecretFile string
	LLMAPIKey        string
	MembersCSVFile   string
	DatabaseFile     string
	TokenFile        string
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
		ClientSecretFile: getEnv("CLIENT_SECRET_FILE", "data/client_secret.json"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		MembersCSVFile:   getEnv("MEMBERS_CSV_FILE", "data/members.csv"),
		DatabaseFile:     getEnv("DATABASE_FILE", "data/comments.db"),
		TokenFile:        getEnv("TOKEN_FILE", "data/token.json"),
	}

	if appConfig.LLMAPIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY não configurada")
	}

	// Initialize database
	if err := database.InitDB(); err != nil {
		return nil, fmt.Errorf("erro ao inicializar o banco de dados: %w", err)
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

	// Transcription DB (PostgreSQL, optional)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		pgDB, err := sql.Open("postgres", dbURL)
		if err != nil {
			ui.Warning(fmt.Sprintf("Não foi possível conectar ao banco de transcrições: %v", err))
		} else if err := pgDB.Ping(); err != nil {
			ui.Warning(fmt.Sprintf("Banco de transcrições inacessível: %v", err))
			pgDB.Close()
		} else {
			app.TranscriptionDB = pgDB
		}
	} else {
		ui.Warning("DATABASE_URL não configurada — transcrições serão buscadas direto do YouTube.")
	}

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
