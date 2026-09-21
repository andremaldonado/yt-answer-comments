package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"answer-comments/internal/app"
	"answer-comments/internal/database"
	"answer-comments/internal/debuglog"
	"answer-comments/internal/service"
	"answer-comments/internal/ui"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Customize flag usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "YouTube Answer Comments - Assistente inteligente para responder comentários\n\n")
		fmt.Fprintf(os.Stderr, "Esta ferramenta monitora comentários não respondidos no seu canal do YouTube\n")
		fmt.Fprintf(os.Stderr, "e sugere respostas usando IA (Gemini), considerando o contexto do vídeo,\n")
		fmt.Fprintf(os.Stderr, "histórico de interações e respostas anteriores similares.\n\n")
		fmt.Fprintf(os.Stderr, "USO:\n")
		fmt.Fprintf(os.Stderr, "  answer-comments [opções]\n\n")
		fmt.Fprintf(os.Stderr, "OPÇÕES:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nREQUISITOS:\n")
		fmt.Fprintf(os.Stderr, "  - client_secret.json: Credenciais OAuth2 do YouTube API\n")
		fmt.Fprintf(os.Stderr, "  - GEMINI_API_KEY: Variável de ambiente com a chave da API Gemini\n")
		fmt.Fprintf(os.Stderr, "  - members.csv (opcional): Lista de membros do canal\n\n")
		fmt.Fprintf(os.Stderr, "EXEMPLOS:\n")
		fmt.Fprintf(os.Stderr, "  answer-comments              # Modo padrão com sugestões da IA\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -d           # Modo de debug (salva arquivo de debug)\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -m           # Modo manual (sem sugestões)\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -a           # Modo automático (publica sem confirmação)\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -t           # Usa transcrição dos vídeos como contexto\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -a -t        # Combina modo automático com transcrição\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -M           # Processa comentários de membros do canal\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -P           # Exibe o perfil conhecido do autor antes do comentário\n\n")
	}

	// Parse command line flags
	manualMode := flag.Bool("manual", false, "Modo manual: pula a sugestão da LLM e força edição manual de todas as respostas")
	flag.BoolVar(manualMode, "m", false, "Atalho para --manual")
	autoAnswerMode := flag.Bool("auto", false, "Modo auto-resposta: todas as respostas sugeridas e com alto nível de confiança pela LLM serão publicadas automaticamente sem confirmação")
	flag.BoolVar(autoAnswerMode, "a", false, "Atalho para --auto")
	transcriptionMode := flag.Bool("transcription", false, "Modo transcrição: usa a transcrição automática do vídeo como contexto para a LLM (exceto para comentários de Saudação/Agradecimento)")
	flag.BoolVar(transcriptionMode, "t", false, "Atalho para --transcription")
	debugMode := flag.Bool("debug", false, "Ativa logging de debug em debug.log (ou caminho configurado com --debug-log)")
	flag.BoolVar(debugMode, "d", false, "Atalho para --debug")
	debugLogPath := flag.String("debug-log", "debug.log", "Caminho do arquivo de log de debug (requer --debug)")
	membersMode := flag.Bool("members", false, "Habilita processamento de comentários de membros do canal (por padrão são ignorados)")
	flag.BoolVar(membersMode, "M", false, "Atalho para --members")
	showProfile := flag.Bool("show-profile", false, "Exibe na tela o perfil conhecido do autor (fatos aprendidos em interações anteriores) antes do comentário")
	flag.BoolVar(showProfile, "P", false, "Atalho para --show-profile")
	migrateSQLite := flag.String("migrate-sqlite", "", "Migra os dados de um arquivo SQLite legado (ex: data/comments.db) para o Postgres e sai")
	flag.Parse()

	if *migrateSQLite != "" {
		runMigration(*migrateSQLite)
		return
	}

	if *debugMode {
		if err := debuglog.Init(*debugLogPath); err != nil {
			log.Printf("Aviso: %v", err)
		}
	}

	ui.ClearScreen()

	if *manualMode {
		ui.PrintModeBanner("✏️", "Modo Manual Ativado", "Todas as respostas deverão ser editadas manualmente.", ui.FgBrightYellow)
	}
	if *autoAnswerMode {
		ui.PrintModeBanner("🤖", "Modo Auto-Resposta Ativado", "Respostas com alto nível de confiança serão publicadas automaticamente.", ui.FgBrightGreen)
	}
	if *transcriptionMode {
		ui.PrintModeBanner("🎙️", "Modo Transcrição Ativado", "A transcrição dos vídeos será usada como contexto para a LLM.", ui.FgBrightCyan)
	}
	if *debugMode {
		ui.PrintModeBanner("⚠️", "Modo de Debug Ativado", "Arquivo de debug será salvo.", ui.FgBrightRed)
	}
	if *membersMode {
		ui.PrintModeBanner("⭐", "Modo Membros Ativado", "Comentários de membros serão processados normalmente.", ui.FgBrightMagenta)
	} else {
		ui.PrintModeBanner("⭐", "Membros Ignorados", "Comentários de membros serão pulados automaticamente.", ui.FgBrightYellow)
	}
	if *showProfile {
		ui.PrintModeBanner("🧠", "Exibição de Perfil Ativada", "O perfil conhecido do autor será exibido antes do comentário.", ui.FgBrightMagenta)
	}

	ctx := context.Background()

	// Initialize App
	myApp, err := app.NewApp(ctx, *transcriptionMode)
	if err != nil {
		log.Printf("Erro ao inicializar aplicação: %v", err)
		os.Exit(1)
	}
	defer myApp.Close()

	ui.Success("Autenticado com sucesso! ID do seu canal: " + myApp.ChannelID)

	// Initialize Service
	commentService := service.NewCommentService(myApp)

	// Start processing
	opts := service.AnswerOptions{
		ManualMode:        *manualMode,
		AutoAnswerMode:    *autoAnswerMode,
		TranscriptionMode: *transcriptionMode,
		MembersMode:       *membersMode,
		ShowProfile:       *showProfile,
	}

	if err := commentService.ProcessComments(ctx, opts); err != nil {
		log.Printf("Erro durante o processamento: %v", err)
		os.Exit(1)
	}
}

// runMigration migra os dados de um arquivo SQLite legado para o Postgres (COMMENTS_DATABASE_URL) e encerra o programa.
func runMigration(sqlitePath string) {
	if err := godotenv.Load("config.env"); err != nil {
		log.Printf("Aviso: Arquivo .env não encontrado. Usando variáveis de ambiente do sistema.")
	}

	pgURL := os.Getenv("COMMENTS_DATABASE_URL")
	if pgURL == "" {
		log.Fatal("COMMENTS_DATABASE_URL não configurada")
	}

	pgDB, err := sql.Open("postgres", pgURL)
	if err != nil {
		log.Fatalf("erro ao conectar ao Postgres: %v", err)
	}
	defer pgDB.Close()

	if err := database.InitDB(pgURL); err != nil {
		log.Fatalf("erro ao preparar a tabela comments no Postgres: %v", err)
	}

	migrated, err := database.MigrateFromSQLite(sqlitePath, pgDB)
	if err != nil {
		log.Fatalf("erro durante a migração: %v", err)
	}

	fmt.Printf("Migração concluída: %d comentários migrados para o Postgres.\n", migrated)
}
