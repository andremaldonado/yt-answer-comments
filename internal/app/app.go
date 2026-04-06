package app

import (
	"context"
	"fmt"
	"log"
	"os"

	"answer-comments/internal/database"
	yt "answer-comments/internal/youtube"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
	"google.golang.org/genai"
)

type Config struct {
	ClientSecretFile string
	GeminiAPIKey     string
	MembersCSVFile   string
	DatabaseFile     string
	TokenFile        string
}

type App struct {
	Config       *Config
	YTService    *youtube.Service
	GeminiClient *genai.Client
	ChannelID    string
}

func NewApp(ctx context.Context, transcriptionMode bool) (*App, error) {
	// Load config.env file
	if err := godotenv.Load("config.env"); err != nil {
		log.Printf("Aviso: Arquivo .env não encontrado. Usando variáveis de ambiente do sistema.")
	}

	appConfig := &Config{
		ClientSecretFile: getEnv("CLIENT_SECRET_FILE", "data/client_secret.json"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		MembersCSVFile:   getEnv("MEMBERS_CSV_FILE", "data/members.csv"),
		DatabaseFile:     getEnv("DATABASE_FILE", "data/comments.db"),
		TokenFile:        getEnv("TOKEN_FILE", "data/token.json"),
	}

	if appConfig.GeminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY não configurada")
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

	// Gemini Client
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  appConfig.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar cliente Gemini: %w", err)
	}

	return &App{
		Config:       appConfig,
		YTService:    service,
		GeminiClient: geminiClient,
		ChannelID:    channelID,
	}, nil
}

func (a *App) Close() {
	database.CloseDB()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
