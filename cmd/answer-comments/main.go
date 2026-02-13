package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
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

	var pageToken string

	// Infinite loop to continuously check for new comments
	for {
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

				// Show a loading screen while processing the comment
				stopLoading := func() {}
				if closer, err := runLoadingScreen("Processando comentário..."); err != nil {
					log.Printf("⚠️ Não foi possível exibir tela de carregamento: %v", err)
				} else {
					stopLoading = func() {
						if closer != nil {
							if err := closer(); err != nil {
								log.Printf("⚠️ Erro ao fechar tela de carregamento: %v", err)
							}
						}
						closer = nil
					}
				}

				isMember := membersMap["https://www.youtube.com/channel/"+comment.Snippet.AuthorChannelId.Value] // String adjusted to match full URL, that is how it appears in the CSV

				// Find the video title and description
				videoCall := service.Videos.List([]string{"snippet"}).Id(comment.Snippet.VideoId)
				videoResp, err := videoCall.Do()
				videoTitle := "[Não foi possível obter o título]"
				videoDescription := "[Não foi possível obter a descrição]"
				if err == nil && len(videoResp.Items) > 0 {
					videoTitle = videoResp.Items[0].Snippet.Title
					videoDescription = videoResp.Items[0].Snippet.Description
				}

				brTime := commentPublishedAt.In(time.FixedZone("BRT", -3*60*60))
				publishedAt := brTime.Format("02/01/2006 às 15:04")
				commentText := comment.Snippet.TextDisplay

				sentiment, err := llm.AnalyzeComment(ctx, comment.Snippet.TextOriginal, geminiClient)
				if err != nil {
					stopLoading()
					fmt.Println("⚠️ Não foi possível analisar o sentimento deste comentário, pulando para o próximo.")
					fmt.Println("Error:", err)
					os.Exit(1)
					continue // Jump to the next comment
				}

				overviewNotes := []string{}
				if *manualMode {
					overviewNotes = append(overviewNotes, "Modo manual ativado: gere e edite sua própria resposta.")
				}

				transcriptStatus := "ℹ️ Transcrição desativada para este comentário."
				if *transcriptionMode {
					transcriptStatus = "ℹ️ Transcrição disponível, aguardando análise."
					if sentiment.Tema == "Saudação/Agradecimento" {
						transcriptStatus = "ℹ️ Transcrição ignorada para Saudação/Agradecimento."
					}
				}

				var videoTranscript string

				var answer, suggestedAnswer, input string

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
				authorHistoryCount := len(authorHistory)
				pastAnswersCount := len(pastAnswers)

				shouldSuggestAnswer := !*manualMode && sentiment.Sentimento != "negativo" && sentiment.Nota >= 3
				if !shouldSuggestAnswer && !*manualMode {
					overviewNotes = append(overviewNotes, "Sugestão automática desativada para este comentário (sentimento negativo ou baixa confiança).")
				}
				if shouldSuggestAnswer {

					// Get video transcription if flag is set
					if *transcriptionMode && sentiment.Tema != "Saudação/Agradecimento" {
						transcriptStatus = "⏳ Buscando transcrição do vídeo..."
						videoTranscript, err = yt.GetVideoTranscription(ctx, service, comment.Snippet.VideoId)
						if err != nil {
							log.Printf("⚠️ Não foi possível obter a transcrição: %v", err)
							transcriptStatus = "⚠️ Transcrição não disponível, continuando sem ela."
						} else {
							transcriptStatus = fmt.Sprintf("✅ Transcrição obtida com sucesso (%d caracteres)", len(videoTranscript))
						}
					}

					// Suggest answer using Gemini
					suggestedAnswer, err = llm.SuggestAnswer(ctx, sentiment.Sentimento == "negativo", comment.Snippet.TextOriginal, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, pastAnswers, geminiClient)

					if suggestedAnswer == "" || err != nil {
						stopLoading()
						fmt.Println("⚠️ Não foi possível gerar uma sugestão de resposta para este comentário. Seguindo para o próximo comentário.")
						fmt.Println("Error:", err)
						fmt.Println("")
						continue
					}

					// Show suggested answer and note
					answer = strings.TrimSpace(suggestedAnswer)

					// Auto-approve positive comments with high confidence
					if suggestedAnswer != "" && sentiment.Sentimento == "positivo" && sentiment.Nota >= 4 && *autoAnswerMode {
						input = "S"
						// wait a moment to let user read
						time.Sleep(2 * time.Second)
						fmt.Println("✅ Resposta sugerida será publicada automaticamente devido ao modo auto-resposta.")
						time.Sleep(3 * time.Second)
					}

				}

				stopLoading()
				if input == "" {
					responseData := responseDecisionData{
						VideoTitle:         videoTitle,
						Author:             comment.Snippet.AuthorDisplayName,
						IsMember:           isMember,
						Comment:            commentText,
						SuggestedAnswer:    suggestedAnswer,
						Sentiment:          sentiment.Sentimento,
						Score:              sentiment.Nota,
						Topic:              sentiment.Tema,
						PublishedAt:        publishedAt,
						AuthorHistoryCount: authorHistoryCount,
						PastAnswersCount:   pastAnswersCount,
						TranscriptStatus:   transcriptStatus,
						Notes:              overviewNotes,
						ManualMode:         *manualMode,
					}
					selection, exitApp, err := runResponseDecisionScreen(responseData)
					if err != nil {
						log.Printf("⚠️ Erro ao exibir tela de decisão: %v", err)
						input = "N"
					} else if exitApp {
						fmt.Println("Encerrando a aplicação.")
						return
					} else {
						input = selection
					}
				}

				// If no suggested answer, force edit. Only if not already in manual mode
				if suggestedAnswer == "" && input == "" {
					displayStatus(*autoAnswerMode, "Sugestão indisponível", "⚠️ Optei por não gerar uma resposta automática para este comentário.")
					input = "E"
				}

				switch input {
				case "S":
					err := yt.PublishComment(service, comment.Id, answer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						displayStatus(*autoAnswerMode, "Erro ao publicar", "Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, answer, false); err != nil {
							log.Printf("⚠️ Erro ao salvar resposta no banco de dados: %v", err)
							displayStatus(*autoAnswerMode, "Aviso", "✅ Resposta publicada, mas houve erro ao salvar no histórico local!")

						}
					}
				case "E":
					initialText := strings.TrimSpace(answer)
					editedAnswer, canceled, err := runEditAnswerScreen(initialText)
					if err != nil {
						log.Printf("Falha ao exibir editor de respostas: %v", err)
						displayStatus(*autoAnswerMode, "Erro", "Não foi possível abrir o editor de respostas. Tente novamente mais tarde.")
						break
					}
					if canceled {
						displayStatus(*autoAnswerMode, "Edição cancelada", "Resposta não publicada. Seguindo para o próximo comentário.")
						break
					}
					editedAnswer = strings.TrimSpace(editedAnswer)
					if editedAnswer == "" {
						displayStatus(*autoAnswerMode, "Resposta vazia", "🚫 Resposta vazia. Seguindo para o próximo comentário.")
						break
					}
					answer = editedAnswer
					err = yt.PublishComment(service, comment.Id, editedAnswer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						displayStatus(*autoAnswerMode, "Erro ao publicar", "Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, editedAnswer, true); err != nil {
							log.Printf("⚠️ Erro ao salvar resposta no banco de dados: %v", err)
							displayStatus(*autoAnswerMode, "Aviso", "✅ Resposta editada publicada, mas houve erro ao salvar no histórico local!")

						}
					}
				case "Q":
					displayStatus(*autoAnswerMode, "Encerrando", "Encerrando a aplicação.")
					return
				default:
					displayStatus(*autoAnswerMode, "Aviso", "🚫 Resposta não publicada. Seguindo para o próximo comentário.")
				}
			}
		}

		if !foundUnanswered {
			if pageToken == "" {
				finalMessage := "Não há mais comentários não respondidos em todas as páginas disponíveis.\nEncerrando a aplicação."
				if err := showMessageModal("Sem novos comentários", finalMessage); err != nil {
					log.Printf("⚠️ Não foi possível exibir mensagem final: %v", err)
				}
				return
			}
			exitApp, err := runNextBatchPrompt()
			if err != nil {
				log.Printf("⚠️ Erro ao exibir prompt de próximo lote: %v", err)
				exitApp = true
			}
			if exitApp {
				if err := showMessageModal("Encerrando", "Encerrando a aplicação."); err != nil {
					log.Printf("⚠️ Não foi possível exibir mensagem de encerramento: %v", err)
				}
				return
			}
		}
	}
}

// setupResult holds the initialized services and data needed to run the main application loop, including the YouTube service client, authenticated channel ID, members map, and Gemini client.
type setupResult struct {
	youtubeService *youtube.Service
	channelID      string
	members        map[string]bool
	geminiClient   *genai.Client
}

// runPreparationScreen initializes the application environment, including YouTube service, Gemini client, and loading members. It provides real-time feedback on the setup progress and handles any errors that may occur during initialization. The user can choose to proceed with the setup or exit the application if any critical error occurs.
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

// performSetup initializes the YouTube service, Gemini client, loads members and prepares the environment. It logs progress through the provided logStep callback.
type responseDecisionData struct {
	VideoTitle         string
	Author             string
	IsMember           bool
	Comment            string
	SuggestedAnswer    string
	Sentiment          string
	Score              int
	Topic              string
	PublishedAt        string
	AuthorHistoryCount int
	PastAnswersCount   int
	TranscriptStatus   string
	Notes              []string
	ManualMode         bool
}

// runResponseDecisionScreen displays the comment details, sentiment analysis, and suggested answer (if available), allowing the user to choose whether to publish, edit, skip, or exit.
func runResponseDecisionScreen(data responseDecisionData) (string, bool, error) {
	app := tview.NewApplication()
	var selection string
	var exit bool

	author := data.Author
	if data.IsMember {
		author = fmt.Sprintf("⭐ MEMBRO ⭐ %s", author)
	}
	titleText := tview.Escape(data.VideoTitle)
	authorText := tview.Escape(author)
	sentLabel := data.Sentiment
	if len(sentLabel) > 0 {
		sentLabel = strings.ToUpper(sentLabel[:1]) + sentLabel[1:]
	}
	sentLabelText := tview.Escape(sentLabel)
	transcriptStatus := tview.Escape(data.TranscriptStatus)
	noteLines := make([]string, 0, len(data.Notes))
	for _, note := range data.Notes {
		noteLines = append(noteLines, tview.Escape(note))
	}
	detailText := fmt.Sprintf("Vídeo: %s\nAutor: %s\nPublicado em: %s\nTema: %s\nSentimento: %s (%d)\nHistórico do autor: %d mensagens\nRespostas similares: %d\n%s",
		titleText,
		authorText,
		data.PublishedAt,
		tview.Escape(data.Topic),
		sentLabelText,
		data.Score,
		data.AuthorHistoryCount,
		data.PastAnswersCount,
		transcriptStatus,
	)
	if len(noteLines) > 0 {
		detailText += "\n" + strings.Join(noteLines, "\n")
	}

	detailView := tview.NewTextView()
	detailView.SetDynamicColors(true)
	detailView.SetWrap(true)
	detailView.SetBorder(true)
	detailView.SetTitle("Resumo do comentário")
	detailView.SetChangedFunc(func() { app.Draw() })
	detailView.SetText(detailText)

	commentBox := tview.NewTextView()
	commentBox.SetDynamicColors(false)
	commentBox.SetWrap(true)
	commentBox.SetBorder(true)
	commentBox.SetTitle("Comentário original")
	commentBox.SetChangedFunc(func() { app.Draw() })
	commentBox.SetText(tview.Escape(data.Comment))

	suggestionText := strings.TrimSpace(data.SuggestedAnswer)
	if suggestionText == "" {
		if data.ManualMode {
			suggestionText = "Modo manual: gere sua resposta usando o botão 'Editar'."
		} else {
			suggestionText = "Sem sugestão automática disponível para este comentário."
		}
	}
	suggestionBox := tview.NewTextView()
	suggestionBox.SetDynamicColors(false)
	suggestionBox.SetWrap(true)
	suggestionBox.SetBorder(true)
	suggestionBox.SetTitle("Sugestão da IA")
	suggestionBox.SetChangedFunc(func() { app.Draw() })
	suggestionBox.SetText(tview.Escape(suggestionText))

	headline := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[::b]Novo comentário não respondido encontrado[-]")

	middleRow := tview.NewFlex().
		AddItem(commentBox, 0, 1, false).
		AddItem(suggestionBox, 0, 1, false)

	body := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(headline, 3, 0, false).
		AddItem(detailView, 0, 2, false).
		AddItem(middleRow, 0, 3, false)

	allowPublish := strings.TrimSpace(data.SuggestedAnswer) != "" && !data.ManualMode
	buttonConfigs := []struct {
		label string
		value string
		show  bool
	}{
		{"Publicar", "S", allowPublish},
		{"Editar", "E", true},
		{"Pular", "N", !data.ManualMode},
		{"Sair", "", true},
	}

	buttonRow := tview.NewFlex()
	var focusables []tview.Primitive
	for _, cfg := range buttonConfigs {
		if !cfg.show {
			continue
		}
		btn := tview.NewButton(cfg.label)
		value := cfg.value
		btn.SetSelectedFunc(func() {
			if value == "" {
				exit = true
			} else {
				selection = value
			}
			app.Stop()
		})
		buttonRow.AddItem(btn, 0, 1, len(focusables) == 0)
		focusables = append(focusables, btn)
	}
	if len(focusables) == 0 {
		// fallback to edit
		btn := tview.NewButton("Editar")
		btn.SetSelectedFunc(func() {
			selection = "E"
			app.Stop()
		})
		buttonRow.AddItem(btn, 0, 1, true)
		focusables = append(focusables, btn)
	}

	currentFocus := 0
	updateFocus := func(next int) {
		if len(focusables) == 0 {
			return
		}
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
		AddItem(body, 0, 1, false).
		AddItem(buttonRow, 3, 0, true)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			updateFocus(currentFocus + 1)
			return nil
		case tcell.KeyBacktab, tcell.KeyLeft:
			updateFocus(currentFocus - 1)
			return nil
		case tcell.KeyEscape:
			exit = true
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(layout, true).SetFocus(focusables[0]).Run(); err != nil {
		return "", false, err
	}

	if exit {
		return "", true, nil
	}
	return selection, false, nil
}

// displayStatus shows a modal message with the given title and text. If autoMode is true, it will not display anything to avoid interrupting the flow.
func displayStatus(autoMode bool, title, text string) {
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if title == "" && text == "" {
		return
	}
	if autoMode {
		return
	}
	if err := showMessageModal(title, text); err != nil {
		log.Printf("⚠️ Não foi possível exibir mensagem: %v", err)
	}
}

// showMessageModal displays a simple modal with a title and message, waiting for user confirmation to close.
func showMessageModal(title, text string) error {
	return runMessageModal(title, text, 0)
}

// runMessageModal displays a simple modal with a title and message. If duration is greater than 0, it will automatically close after the specified time.
func runMessageModal(title, text string, duration time.Duration) error {
	app := tview.NewApplication()
	safeTitle := tview.Escape(strings.TrimSpace(title))
	safeText := tview.Escape(strings.TrimSpace(text))
	var content string
	if safeTitle != "" {
		content = fmt.Sprintf("[::b]%s[-]\n\n%s", safeTitle, safeText)
	} else {
		content = safeText
	}
	if duration > 0 {
		countdown := fmt.Sprintf("\n\n[gray]Esta janela será fechada automaticamente em %d segundos.[-]", int(duration.Seconds()))
		content += countdown
	}
	modal := tview.NewModal().
		SetText(content).
		AddButtons([]string{"OK"})
	var timer *time.Timer
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if timer != nil && !timer.Stop() {
			<-timer.C
		}
		app.Stop()
	})
	if duration > 0 {
		timer = time.NewTimer(duration)
		go func() {
			<-timer.C
			app.Stop()
		}()
	}
	return app.SetRoot(modal, true).Run()
}

// runLoadingScreen displays a simple loading screen with a message and returns a function to stop it.
// The returned function should be called to dismiss the loading screen before presenting another UI.
func runLoadingScreen(message string) (func() error, error) {
	app := tview.NewApplication()
	safeMessage := tview.Escape(strings.TrimSpace(message))
	content := fmt.Sprintf("[yellow]%s[-]\n\n[gray]Por favor, aguarde...[-]", safeMessage)
	modal := tview.NewModal().
		SetText(content)
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.SetRoot(modal, true).Run()
	}()
	var once sync.Once
	var result error
	stop := func() error {
		once.Do(func() {
			app.Stop()
			result = <-errChan
		})
		return result
	}
	return stop, nil
}

// runNextBatchPrompt shows a prompt when there are no more unanswered comments in the current batch, asking the user if they want to fetch the next batch or exit the application.
func runNextBatchPrompt() (bool, error) {
	app := tview.NewApplication()
	var exit bool
	header := tview.Escape("Não há mais comentários não respondidos neste lote.")
	body := tview.Escape("Deseja buscar o próximo lote ou encerrar a aplicação?")
	content := fmt.Sprintf("[::b]%s[-]\n\n%s", header, body)
	modal := tview.NewModal().
		SetText(content).
		AddButtons([]string{"Buscar próximo lote", "Sair"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Sair" {
				exit = true
			}
			app.Stop()
		})
	return exit, app.SetRoot(modal, true).Run()
}

// runEditAnswerScreen opens a text editor interface for the user to edit the suggested answer or create a new one.
func runEditAnswerScreen(initial string) (string, bool, error) {
	app := tview.NewApplication()
	var result string
	var canceled bool
	initialText := strings.TrimSpace(initial)

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetText("[::b]Edite a resposta abaixo[-]\nUse Tab para navegar entre os campos e botões. Pressione Esc para cancelar e Ctrl+S para salvar.")

	originalView := tview.NewTextView()
	originalView.SetDynamicColors(false)
	originalView.SetWrap(true)
	originalView.SetBorder(true)
	originalView.SetTitle("Sugestão atual")
	if initialText == "" {
		originalView.SetText("Nenhuma sugestão disponível para este comentário.")
	} else {
		originalView.SetText(initialText)
	}

	textArea := tview.NewTextArea().
		SetLabel("Resposta").
		SetPlaceholder("Digite a resposta que deseja publicar...").
		SetWrap(true).
		SetWordWrap(true)
	textArea.SetText("", false)
	textArea.SetFinishedFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			canceled = true
			app.Stop()
		case tcell.KeyCtrlS:
			result = textArea.GetText()
			app.Stop()
		}
	})

	form := tview.NewForm().
		AddFormItem(textArea)
	form.AddButton("Salvar", func() {
		result = textArea.GetText()
		app.Stop()
	})
	form.AddButton("Cancelar", func() {
		canceled = true
		app.Stop()
	})
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetBorder(true).SetTitle("Editar resposta")

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(info, 3, 0, false).
		AddItem(originalView, 0, 1, false).
		AddItem(form, 0, 2, true)

	if err := app.SetRoot(layout, true).SetFocus(textArea).Run(); err != nil {
		return "", false, err
	}
	if canceled {
		return "", true, nil
	}
	return strings.TrimSpace(result), false, nil
}

// performSetup initializes the application environment, including database connection, YouTube API client, and Gemini API client.
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
