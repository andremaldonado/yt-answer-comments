package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"answer-comments/internal/app"
	"answer-comments/internal/service"
	"answer-comments/internal/ui"
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
		fmt.Fprintf(os.Stderr, "  answer-comments -m           # Modo manual (sem sugestões)\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -a           # Modo automático (publica sem confirmação)\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -t           # Usa transcrição dos vídeos como contexto\n")
		fmt.Fprintf(os.Stderr, "  answer-comments -a -t        # Combina modo automático com transcrição\n\n")
	}

	// Parse command line flags
	manualMode := flag.Bool("manual", false, "Modo manual: pula a sugestão da LLM e força edição manual de todas as respostas")
	flag.BoolVar(manualMode, "m", false, "Atalho para --manual")
	autoAnswerMode := flag.Bool("auto", false, "Modo auto-resposta: todas as respostas sugeridas e com alto nível de confiança pela LLM serão publicadas automaticamente sem confirmação")
	flag.BoolVar(autoAnswerMode, "a", false, "Atalho para --auto")
	transcriptionMode := flag.Bool("transcription", false, "Modo transcrição: usa a transcrição automática do vídeo como contexto para a LLM (exceto para comentários de Saudação/Agradecimento)")
	flag.BoolVar(transcriptionMode, "t", false, "Atalho para --transcription")
	flag.Parse()

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
	}

	if err := commentService.ProcessComments(ctx, opts); err != nil {
		log.Printf("Erro durante o processamento: %v", err)
		os.Exit(1)
	}
}
