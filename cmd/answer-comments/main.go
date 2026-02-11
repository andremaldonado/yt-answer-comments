package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"answer-comments/internal/database"
	"answer-comments/internal/llm"
	yt "answer-comments/internal/youtube"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"google.golang.org/genai"
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

	ctx := context.Background()
	setup, exitSetup, err := runPreparationScreen(ctx, *manualMode, *autoAnswerMode, *transcriptionMode)
	if err != nil {
		log.Printf("Erro durante a preparação: %v", err)
		os.Exit(1)
	}
	if exitSetup {
		fmt.Println("Encerrando a aplicação.")
		return
	}
	defer database.CloseDB()

	service := setup.youtubeService
	myChannelId := setup.channelID
	membersMap := setup.members
	geminiClient := setup.geminiClient

	// Clear the terminal screen (works on most terminals)
	fmt.Print("\033[H\033[2J")

	// Prepare to read user input
	reader := bufio.NewReader(os.Stdin)
	var pageToken string

	// Ask for user confirmation before starting
	fmt.Print("-> Pressione Enter para iniciar a verificação de novos comentários não respondidos...")
	_, _ = reader.ReadString('\n')

	// Infinite loop to continuously check for new comments
	for {
		fmt.Println("")
		fmt.Println("------------------------------------------------------------------")
		fmt.Println("Buscando novos comentários não respondidos...")
		fmt.Println("------------------------------------------------------------------")

		call := service.CommentThreads.List([]string{"snippet,replies"}).
			AllThreadsRelatedToChannelId(myChannelId).
			Order("time").
			PageToken(pageToken).
			MaxResults(25)

		response, err := call.Do()
		if err != nil {
			log.Printf("Erro ao buscar os comentários: %v", err)
			return
		}

		pageToken = response.NextPageToken // Token update for next iteration

		foundUnanswered := false

		for _, item := range response.Items {
			comment := item.Snippet.TopLevelComment
			commentPublishedAt, _ := time.Parse(time.RFC3339, comment.Snippet.PublishedAt)

			isAnsweredByMe := false
			if item.Replies != nil {
				for _, reply := range item.Replies.Comments {
					if reply.Snippet.AuthorChannelId.Value == myChannelId {
						isAnsweredByMe = true
						break
					}
				}
			}

			if !isAnsweredByMe {
				foundUnanswered = true

				isMember := membersMap["https://www.youtube.com/channel/"+comment.Snippet.AuthorChannelId.Value] // String adjusted to match full URL, that is how it appears in the CSV
				authorPrefix := ""
				if isMember {
					authorPrefix = "⭐ MEMBRO ⭐ "
				}

				// Find the video title and description
				videoCall := service.Videos.List([]string{"snippet"}).Id(comment.Snippet.VideoId)
				videoResp, err := videoCall.Do()
				videoTitle := "[Não foi possível obter o título]"
				videoDescription := "[Não foi possível obter a descrição]"
				if err == nil && len(videoResp.Items) > 0 {
					videoTitle = videoResp.Items[0].Snippet.Title
					videoDescription = videoResp.Items[0].Snippet.Description
				}

				// Clear the terminal screen (works on most terminals)
				fmt.Print("\033[H\033[2J")

				// Show screen title
				fmt.Println("------------------------------------------------------------------")
				fmt.Println("             Novo comentário não respondido encontrado            ")
				fmt.Println("------------------------------------------------------------------")
				fmt.Println("")

				// Show comment details
				brTime := commentPublishedAt.In(time.FixedZone("BRT", -3*60*60))
				fmt.Println("# Detalhes do comentário")
				fmt.Printf("Título do vídeo: %s\n", videoTitle)
				fmt.Printf("%sAutor: %s (Publicado em: %s)\n", authorPrefix, comment.Snippet.AuthorDisplayName, brTime.Format("02/01/2006 às 15:04"))
				fmt.Printf("Comentário: %s\n\n", comment.Snippet.TextDisplay)

				// Analyze comment with Gemini
				fmt.Print("# Análise do comentário: ")
				sentiment, err := llm.AnalyzeComment(ctx, comment.Snippet.TextOriginal, geminiClient)
				if err != nil {
					fmt.Println("⚠️ Não foi possível analisar o sentimento deste comentário, pulando para o próximo.")
					fmt.Println("Error:", err)
					os.Exit(1)
					continue // Jump to the next comment
				}
				fmt.Printf(" %s |", sentiment.Sentimento)
				fmt.Printf(" %d |", sentiment.Nota)
				fmt.Printf(" %s\n\n", sentiment.Tema)

				var answer, suggestedAnswer, input string

				// In manual mode, do not generate suggested answer
				if *manualMode {
					input = "E" // Forces manual edit mode
					suggestedAnswer = ""
				}

				// Buscar exemplos anteriores para RAG
				pastAnswers, err := database.GetPreviousAnswersByContext(sentiment.Tema, sentiment.Sentimento, 5)
				if err != nil {
					log.Printf("⚠️ Erro ao buscar histórico de RAG: %v", err)
					pastAnswers = nil
				}

				// Buscar histórico do autor
				authorHistory, err := database.GetLastComments(comment.Snippet.AuthorDisplayName, 10)
				if err != nil {
					log.Printf("⚠️ Erro ao buscar histórico de comentários: %v", err)
					authorHistory = nil
				}

				shouldSuggestAnswer := !*manualMode && sentiment.Sentimento != "negativo" && sentiment.Nota >= 3
				if shouldSuggestAnswer {

					// Get video transcription if flag is set
					var videoTranscript string
					if *transcriptionMode && sentiment.Tema != "Saudação/Agradecimento" {
						fmt.Println("# Transcrição do vídeo")
						fmt.Printf("Buscando transcrição do vídeo...\n")
						videoTranscript, err = yt.GetVideoTranscription(ctx, service, comment.Snippet.VideoId)
						if err != nil {
							log.Printf("⚠️ Não foi possível obter a transcrição: %v", err)
							fmt.Println("⚠️ Transcrição não disponível, continuando sem ela.")
						} else {
							fmt.Printf("✅ Transcrição obtida com sucesso (%d caracteres)\n\n", len(videoTranscript))
						}
					}

					fmt.Println("# RAG")

					// If there is history, show to user
					if len(authorHistory) > 0 {
						fmt.Printf("✅ %d mensagens encontradas no histórico de interações anteriores com esta pessoa.\n", len(authorHistory))
					}

					// If there is similar previous answers, show to user
					if len(pastAnswers) > 0 {
						fmt.Printf("✅ %d respostas similares encontradas no histórico.\n", len(pastAnswers))
					}

					// Suggest answer using Gemini
					fmt.Println("\n# Sugestão de resposta")
					suggestedAnswer, err = llm.SuggestAnswer(ctx, sentiment.Sentimento == "negativo", comment.Snippet.TextOriginal, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, pastAnswers, geminiClient)

					if suggestedAnswer == "" || err != nil {
						fmt.Println("⚠️ Não foi possível gerar uma sugestão de resposta para este comentário. Seguindo para o próximo comentário.")
						fmt.Println("Error:", err)
						fmt.Println("")
						continue
					}

					// Show suggested answer and note
					answer = strings.TrimSpace(suggestedAnswer)
					fmt.Printf("%s\n", answer)

					// Auto-approve positive comments with high confidence
					if suggestedAnswer != "" && sentiment.Sentimento == "positivo" && sentiment.Nota >= 4 && *autoAnswerMode {
						input = "S"
						// wait a moment to let user read
						time.Sleep(2 * time.Second)
						fmt.Println("✅ Resposta sugerida será publicada automaticamente devido ao modo auto-resposta.")
						time.Sleep(3 * time.Second)
					}

					// If not auto-approved, ask user
					if input == "" {
						fmt.Printf("\nDeseja publicar esta resposta? (S/N/E/Q para sair): ")
						input, _ = reader.ReadString('\n')
						input = strings.TrimSpace(strings.ToUpper(input))
					}
				}

				// If no suggested answer, force edit. Only if not already in manual mode
				if suggestedAnswer == "" && input == "" {
					fmt.Println("⚠️ Optei por não gerar uma resposta automática para este comentário.")
					input = "E"
				}

				switch input {
				case "S":
					err := yt.PublishComment(service, comment.Id, answer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						fmt.Println("Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						// Save to database
						if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, answer, false); err != nil {
							log.Printf("⚠️ Erro ao salvar resposta no banco de dados: %v", err)
							fmt.Println("✅ Resposta publicada, mas houve erro ao salvar no histórico local!")
						} else {
							fmt.Println("✅ Resposta publicada e salva com sucesso!")
						}
					}
				case "E":
					fmt.Print("Digite a resposta que deseja publicar:\n> ")
					editedAnswer, _ := reader.ReadString('\n')
					editedAnswer = strings.TrimSpace(editedAnswer)
					answer = editedAnswer
					if editedAnswer == "" {
						fmt.Println("🚫 Resposta vazia. Seguindo para o próximo comentário.")
						break
					}
					err := yt.PublishComment(service, comment.Id, editedAnswer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						fmt.Println("Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						// Save to database with userAnswered flag
						if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, editedAnswer, true); err != nil {
							log.Printf("⚠️ Erro ao salvar resposta no banco de dados: %v", err)
							fmt.Println("✅ Resposta editada publicada, mas houve erro ao salvar no histórico local!")
						} else {
							fmt.Println("✅ Resposta editada publicada e salva com sucesso!")
						}
					}
				case "Q":
					fmt.Println("Encerrando a aplicação.")
					return
				default:
					fmt.Println("🚫 Resposta não publicada. Seguindo para o próximo comentário.")
				}

				fmt.Println("")
			}
		}

		if !foundUnanswered {
			if pageToken == "" {
				fmt.Println("\nNão há mais comentários não respondidos em todas as páginas disponíveis.")
				fmt.Println("Encerrando a aplicação.")
				return // Exit the application
			} else {
				fmt.Println("\nNão há mais comentários não respondidos neste lote.")
				fmt.Printf("Pressione Enter para buscar o próximo lote de comentários, ou digite 'Q' para sair: ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToUpper(input))
				if input == "Q" {
					fmt.Println("Encerrando a aplicação.")
					return
				}
			}
		}
	}
}

type setupResult struct {
	youtubeService *youtube.Service
	channelID      string
	members        map[string]bool
	geminiClient   *genai.Client
}

func runPreparationScreen(ctx context.Context, manualMode, autoMode, useTranscription bool) (*setupResult, bool, error) {
	setupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	app := tview.NewApplication()
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	fmt.Fprintf(statusView, "[yellow]Preparando ambiente...[-]\n\n")

	startButton := tview.NewButton("Carregando...")
	exitButton := tview.NewButton("Sair")
	var ready bool
	var quit bool
	var setupErr error
	var result *setupResult

	startButton.SetSelectedFunc(func() {
		if !ready {
			return
		}
		app.Stop()
	})
	exitButton.SetSelectedFunc(func() {
		quit = true
		cancel()
		app.Stop()
	})

	buttonRow := tview.NewFlex().
		AddItem(startButton, 0, 1, true).
		AddItem(exitButton, 0, 1, false)
	focusables := []tview.Primitive{startButton, exitButton}
	currentFocus := 0
	updateFocus := func(next int) {
		if next < 0 {
			next = len(focusables) - 1
		} else if next >= len(focusables) {
			next = 0
		}
		currentFocus = next
		app.SetFocus(focusables[currentFocus])
	}
	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(statusView, 0, 1, false).
		AddItem(buttonRow, 3, 0, true)
	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			updateFocus(currentFocus + 1)
			return nil
		case tcell.KeyBacktab:
			updateFocus(currentFocus - 1)
			return nil
		case tcell.KeyRight:
			updateFocus(currentFocus + 1)
			return nil
		case tcell.KeyLeft:
			updateFocus(currentFocus - 1)
			return nil
		}
		return event
	})

	logStep := func(msg string) {
		app.QueueUpdateDraw(func() {
			fmt.Fprintf(statusView, "%s\n", msg)
		})
	}

	go func() {
		res, err := performSetup(setupCtx, useTranscription, logStep)
		if err != nil {
			setupErr = err
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(statusView, "[red]Erro: %v[-]\nUse 'Sair' para encerrar.\n", err)
				startButton.SetLabel("Indisponível")
			})
			return
		}
		result = res
		app.QueueUpdateDraw(func() {
			ready = true
			startButton.SetLabel("Iniciar")
			fmt.Fprintf(statusView, "\n[green]Dependências carregadas![-]\nPressione 'Iniciar' para continuar ou 'Sair' para fechar.\n")
			fmt.Fprintf(statusView, "\n[blue]Resumo do ambiente[-]\n")
			fmt.Fprintf(statusView, "• Canal autenticado: %s\n", result.channelID)
			fmt.Fprintf(statusView, "• Membros carregados: %d\n", len(result.members))
			fmt.Fprintf(statusView, "\n[blue]Modo de execução[-]\n")
			if manualMode {
				fmt.Fprintf(statusView, "⚠️ Modo manual ativado: todas as respostas serão editadas manualmente.\n")
			} else {
				fmt.Fprintf(statusView, "✅ Modo assistido: respostas serão sugeridas pela IA.\n")
			}
			if autoMode {
				fmt.Fprintf(statusView, "⚠️ Auto-resposta ligada: respostas positivas com alta confiança serão publicadas automaticamente.\n")
			} else {
				fmt.Fprintf(statusView, "✅ Publicação manual: cada resposta será confirmada antes de enviar.\n")
			}
			if useTranscription {
				fmt.Fprintf(statusView, "✅ Transcrição ativa: contexto dos vídeos será usado quando aplicável.\n")
			} else {
				fmt.Fprintf(statusView, "ℹ️ Transcrição desativada: apenas título e descrição do vídeo serão considerados.\n")
			}
		})
	}()

	if err := app.SetRoot(layout, true).SetFocus(startButton).Run(); err != nil {
		return nil, false, err
	}

	if quit {
		return nil, true, nil
	}
	if setupErr != nil {
		return nil, false, setupErr
	}
	return result, false, nil
}

func performSetup(ctx context.Context, useTranscription bool, logStep func(string)) (*setupResult, error) {
	if logStep == nil {
		logStep = func(string) {}
	}
	logStep("Inicializando banco de dados...")
	if err := database.InitDB(); err != nil {
		return nil, err
	}
	cleanupOnError := func() {
		database.CloseDB()
	}
	logStep("Banco de dados pronto.")

	logStep("Lendo credenciais OAuth...")
	creds, err := os.ReadFile("client_secret.json")
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	logStep("Credenciais carregadas.")

	scopes := []string{yt.YoutubeForceSslScope, yt.YoutubeChannelMembershipsCreatorScope}
	if useTranscription {
		scopes = append(scopes, youtube.YoutubeScope)
	}
	logStep("Configurando OAuth...")
	config, err := google.ConfigFromJSON(creds, scopes...)
	if err != nil {
		cleanupOnError()
		return nil, err
	}

	logStep("Obtendo cliente autenticado do YouTube...")
	client, err := yt.GetYoutubeClient(config)
	if err != nil {
		cleanupOnError()
		return nil, err
	}

	logStep("Criando serviço do YouTube...")
	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		cleanupOnError()
		return nil, err
	}

	logStep("Validando canal autenticado...")
	channelResponse, err := service.Channels.List([]string{"id"}).Mine(true).Do()
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	if len(channelResponse.Items) == 0 {
		cleanupOnError()
		return nil, fmt.Errorf("não foi possível encontrar o ID do canal do usuário autenticado")
	}
	channelID := channelResponse.Items[0].Id
	logStep(fmt.Sprintf("Canal autenticado: %s", channelID))

	logStep("Carregando lista de membros...")
	membersMap, err := loadMembersFromCSV("members.csv")
	if err != nil {
		logStep(fmt.Sprintf("[yellow]Aviso: não foi possível carregar members.csv: %v[-]", err))
		membersMap = make(map[string]bool)
	} else {
		logStep(fmt.Sprintf("%d membros carregados.", len(membersMap)))
	}

	logStep("Validando GEMINI_API_KEY...")
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		cleanupOnError()
		return nil, fmt.Errorf("a variável de ambiente GEMINI_API_KEY não está configurada")
	}
	logStep("Criando cliente Gemini...")
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  geminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	logStep("Cliente Gemini pronto.")

	return &setupResult{
		youtubeService: service,
		channelID:      channelID,
		members:        membersMap,
		geminiClient:   geminiClient,
	}, nil
}

// loadMembersFromCSV reads a CSV file containing channel members and returns a map of member channel IDs.
// The map is used for quick lookup (O(1) on average).
func loadMembersFromCSV(filename string) (map[string]bool, error) {
	// open the CSV file
	file, err := os.Open(filename)
	if err != nil {
		// Returns an empty map if the file does not exist, so the program does not break.
		if os.IsNotExist(err) {
			fmt.Printf("Aviso: Arquivo de membros '%s' não encontrado. A identificação de membros estará desativada.\n", filename)
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("erro ao abrir o arquivo de membros: %w", err)
	}
	defer file.Close()

	// Check if the file is outdated (more than 10 days old)
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter informações do arquivo de membros: %w", err)
	}
	if time.Since(fileInfo.ModTime()) > 10*24*time.Hour {
		fmt.Printf("ATENÇÃO: O arquivo de membros '%s' está desatualizado (última modificação em %s). Considere atualizá-lo.\n", filename, fileInfo.ModTime().Format("02/01/2006"))
	}

	// Create a CSV reader
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler o arquivo de membros: %w", err)
	}

	members := make(map[string]bool)
	if len(records) > 1 { // Jump header
		// Assuming the Channel ID is in the first column (index 0)
		// IMPORTANT: Check your CSV file to confirm the correct column!
		for _, record := range records[1:] {
			if len(record) > 0 {
				channelId := record[1]
				members[channelId] = true
			}
		}
	}

	return members, nil
}
