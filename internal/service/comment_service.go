package service

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"answer-comments/internal/app"
	"answer-comments/internal/database"
	"answer-comments/internal/debuglog"
	"answer-comments/internal/llm"
	"answer-comments/internal/models"
	"answer-comments/internal/ui"
	yt "answer-comments/internal/youtube"

	"google.golang.org/api/youtube/v3"
)

type CommentService struct {
	App *app.App
}

func NewCommentService(a *app.App) *CommentService {
	return &CommentService{App: a}
}

type AnswerOptions struct {
	ManualMode        bool
	AutoAnswerMode    bool
	TranscriptionMode bool
}

func (s *CommentService) ProcessComments(ctx context.Context, opts AnswerOptions) error {
	// load members from CSV
	membersMap, err := s.loadMembersFromCSV(s.App.Config.MembersCSVFile)
	if err != nil {
		log.Printf("Não foi possível carregar a lista de membros: %v", err)
	}
	ui.Success(fmt.Sprintf("Carregados %d membros a partir do arquivo.", len(membersMap)))

	reader := bufio.NewReader(os.Stdin)
	var pageToken string

	// Goroutine permanente de leitura de stdin para AutoAnswerMode.
	// Evita o goroutine leak que ocorria quando Countdown expirava e deixava
	// uma goroutine bloqueada em ReadString, roubando Enters subsequentes.
	var stdinCh chan string
	if opts.AutoAnswerMode {
		stdinCh = make(chan string, 1)
		go func() {
			for {
				line, _ := reader.ReadString('\n')
				debuglog.Log("[stdin] leu linha: %q", line)
				stdinCh <- line
				debuglog.Log("[stdin] entregue ao channel")
			}
		}()
	}

	fmt.Println()
	fmt.Printf("  %s→ Pressione Enter para iniciar a verificação de comentários...%s ", ui.FgBrightCyan+ui.Bold, ui.Reset)
	if opts.AutoAnswerMode {
		<-stdinCh
	} else {
		_, _ = reader.ReadString('\n')
	}

	for {
		ui.PrintSearchingBanner()

		call := s.App.YTService.CommentThreads.List([]string{"snippet,replies"}).
			AllThreadsRelatedToChannelId(s.App.ChannelID).
			Order("time").
			PageToken(pageToken).
			MaxResults(25)

		response, err := call.Do()
		if err != nil {
			return fmt.Errorf("erro ao buscar os comentários: %w", err)
		}

		pageToken = response.NextPageToken
		foundUnanswered := false

		for _, item := range response.Items {
			comment := item.Snippet.TopLevelComment
			commentPublishedAt, _ := time.Parse(time.RFC3339, comment.Snippet.PublishedAt)

			isAnsweredByMe := false
			if item.Replies != nil {
				for _, reply := range item.Replies.Comments {
					if reply.Snippet.AuthorChannelId.Value == s.App.ChannelID {
						isAnsweredByMe = true
						break
					}
				}
			}

			if !isAnsweredByMe {
				foundUnanswered = true
				if err := s.handleUnansweredComment(ctx, comment, commentPublishedAt, membersMap, opts, reader, stdinCh); err != nil {
					if isNetworkError(err) {
						debuglog.Log("[network] erro de rede, abortando: %v", err)
						log.Printf("Erro de rede — abortando: %v", err)
						os.Exit(-1)
					}
					log.Printf("Erro ao processar comentário %s: %v", comment.Id, err)
				}
			}
		}

		if !foundUnanswered {
			if pageToken == "" {
				fmt.Println()
				ui.Info("Não há mais comentários não respondidos em todas as páginas disponíveis.")
				return nil
			} else {
				fmt.Println()
				ui.Info("Não há mais comentários não respondidos neste lote.")
				ui.PrintDivider()
				fmt.Printf("\n  %s→ Pressione Enter para buscar o próximo lote, ou digite Q para sair: %s", ui.FgBrightCyan+ui.Bold, ui.Reset)
				debuglog.Log("[próximo-lote] aguardando input autoMode=%v", opts.AutoAnswerMode)
				var input string
				if opts.AutoAnswerMode {
					input = strings.TrimSpace(strings.ToUpper(<-stdinCh))
				} else {
					line, _ := reader.ReadString('\n')
					input = strings.TrimSpace(strings.ToUpper(line))
				}
				debuglog.Log("[próximo-lote] input=%q", input)
				if input == "Q" {
					return nil
				}
			}
		}
	}
}

func (s *CommentService) handleUnansweredComment(ctx context.Context, comment *youtube.Comment, publishedAt time.Time, membersMap map[string]bool, opts AnswerOptions, reader *bufio.Reader, stdinCh chan string) error {
	debuglog.Log("[comment] início — id=%s autor=%q", comment.Id, comment.Snippet.AuthorDisplayName)
	isMember := membersMap["https://www.youtube.com/channel/"+comment.Snippet.AuthorChannelId.Value]

	videoCall := s.App.YTService.Videos.List([]string{"snippet"}).Id(comment.Snippet.VideoId)
	videoResp, err := videoCall.Do()
	videoTitle := "[Não foi possível obter o título]"
	videoDescription := "[Não foi possível obter a descrição]"
	if err == nil && len(videoResp.Items) > 0 {
		videoTitle = videoResp.Items[0].Snippet.Title
		videoDescription = videoResp.Items[0].Snippet.Description
	}

	// ── Header ────────────────────────────────────────────────────────────────
	ui.ClearScreen()
	ui.PrintHeader("Novo comentário não respondido encontrado")

	// ── Comment Details ───────────────────────────────────────────────────────
	brTime := publishedAt.In(time.FixedZone("BRT", -3*60*60))

	authorLine := comment.Snippet.AuthorDisplayName
	if isMember {
		authorLine = ui.MemberBadge() + authorLine
	}

	ui.PrintCommentMeta(videoTitle, authorLine, brTime.Format("02/01/2006 às 15:04"))
	ui.PrintComment(comment.Snippet.TextDisplay)

	// ── Sentiment Analysis ────────────────────────────────────────────────────
	ui.PrintSectionTitle("Análise do comentário")

	sentiment, err := llm.AnalyzeComment(ctx, comment.Snippet.TextOriginal, s.App.GeminiClient)
	if err != nil {
		return fmt.Errorf("erro na análise de sentimento: %w", err)
	}

	fmt.Printf("  %s  %s  %s\n",
		ui.SentimentBadge(sentiment.Sentimento),
		ui.NotaBadge(sentiment.Nota),
		ui.ThemeBadge(sentiment.Tema),
	)

	var answer, suggestedAnswer, input string
	if opts.ManualMode {
		input = "E"
	}

	pastAnswers, err := database.GetPreviousAnswersByContext(sentiment.Tema, sentiment.Sentimento, 5)
	if err != nil {
		log.Printf("Erro ao buscar histórico de RAG: %v", err)
	}

	authorHistory, err := database.GetLastComments(comment.Snippet.AuthorDisplayName, 10)
	if err != nil {
		log.Printf("Erro ao buscar histórico de comentários: %v", err)
	}

	shouldSuggestAnswer := !opts.ManualMode && sentiment.Sentimento != "negativo" && sentiment.Nota >= 3
	debuglog.Log("[comment] sentimento=%s nota=%d tema=%q shouldSuggest=%v manualMode=%v autoMode=%v",
		sentiment.Sentimento, sentiment.Nota, sentiment.Tema, shouldSuggestAnswer, opts.ManualMode, opts.AutoAnswerMode)
	if shouldSuggestAnswer {
		var videoTranscript string
		transcriptLen := 0 // 0 = not fetched, -1 = error, >0 = char count
		if opts.TranscriptionMode && sentiment.Tema != "Saudação/Agradecimento" {
			videoTranscript, err = yt.GetVideoTranscription(ctx, s.App.YTService, comment.Snippet.VideoId)
			if err != nil {
				log.Printf("Não foi possível obter a transcrição: %v", err)
				transcriptLen = -1
			} else {
				transcriptLen = len(videoTranscript)
			}
		}

		ui.PrintSectionTitle("Contexto")
		ui.PrintContextBar(transcriptLen, len(authorHistory), len(pastAnswers))

		ui.PrintSectionTitle("Sugestão de resposta")
		suggestedAnswer, err = llm.SuggestAnswer(ctx, sentiment.Sentimento == "negativo", comment.Snippet.TextOriginal, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, pastAnswers, s.App.GeminiClient)

		if err != nil { //TODO: fazer o if do auto-answer aqui para sair da app quando der erro
			if opts.AutoAnswerMode {
				os.Exit(-1)
			}
			return fmt.Errorf("erro ao sugerir resposta: %w", err)
		}

		if suggestedAnswer == "" {
			ui.Warning("Não foi possível gerar uma sugestão de resposta.")
			return nil
		}

		answer = strings.TrimSpace(suggestedAnswer)
		ui.PrintSuggestedAnswer(answer)

		if sentiment.Sentimento == "positivo" && sentiment.Nota >= 4 && opts.AutoAnswerMode {
			input = "S"
			ui.Success("Resposta sugerida será publicada automaticamente.")

			debuglog.Log("[countdown] início — path=publish")
			completed := ui.Countdown(3*time.Minute, stdinCh, "Publicando em")
			debuglog.Log("[countdown] fim — completed=%v path=publish", completed)
			if !completed {
				input = "E"
			}
		}

		// Se não tem um opção do que fazer
		if input == "" {
			// E está no modo auto resposta, mostra o countdown. Se o usuário não fizar nada, pula, senão deixa Editar
			if opts.AutoAnswerMode {
				debuglog.Log("[countdown] início — path=skip-with-suggestion")
				completed := ui.Countdown(3*time.Minute, stdinCh, "Pulando comentário em")
				debuglog.Log("[countdown] fim — completed=%v path=skip-with-suggestion", completed)
				if !completed {
					input = "E"
				}
			}
			// Se não está no modo autoresposta, mostra o menu de ações
			if !opts.AutoAnswerMode {
				ui.PrintActionMenu()
				input, _ = reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToUpper(input))
			}
		}
	}

	// Se ainda não tem uma mensagem sugerida
	if suggestedAnswer == "" && input == "" {
		// E está no modo auto-resposta, mostra o countdown. Se o usuário não fizer nada, pula, senão deixa Editar
		if opts.AutoAnswerMode {
			debuglog.Log("[countdown] início — path=threshold reason=%q", thresholdReason(sentiment))
			completed := ui.Countdown(3*time.Minute, stdinCh, thresholdReason(sentiment))
			debuglog.Log("[countdown] fim — completed=%v path=threshold", completed)
			if !completed {
				input = "E"
			}
		}
		// Se nao está no modo auto resposta, só mostra que não tem sugestão e manda editar
		if !opts.AutoAnswerMode {
			ui.Warning("Optei por não gerar uma resposta automática.")
			input = "E"
		}
	}

	debuglog.Log("[comment] input final=%q antes do switch", input)
	switch input {
	case "S":
		return s.publishAndSave(comment, &sentiment, answer, false)
	case "E":
		ui.PrintEditPrompt()
		var editedAnswer string
		if opts.AutoAnswerMode {
			editedAnswer = strings.TrimSpace(<-stdinCh)
		} else {
			line, _ := reader.ReadString('\n')
			editedAnswer = strings.TrimSpace(line)
		}
		if editedAnswer == "" {
			ui.Warning("Resposta vazia — comentário ignorado.")
			return nil
		}
		return s.publishAndSave(comment, &sentiment, editedAnswer, true)
	case "Q":
		os.Exit(0)
	default:
		ui.Warning("Resposta não publicada.")
	}

	return nil
}

func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

func thresholdReason(s models.SentimentAnalysis) string {
	if s.Sentimento != "positivo" {
		return fmt.Sprintf("Sentimento %s —", s.Sentimento)
	}
	return fmt.Sprintf("Nota %d (mínimo 4) —", s.Nota)
}

func (s *CommentService) publishAndSave(comment *youtube.Comment, sentiment *models.SentimentAnalysis, answer string, userAnswered bool) error {
	err := yt.PublishComment(s.App.YTService, comment.Id, answer)
	if err != nil {
		return fmt.Errorf("falha ao publicar resposta: %w", err)
	}

	if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, answer, userAnswered); err != nil {
		log.Printf("Erro ao salvar no banco: %v", err)
		ui.Warning("Resposta publicada, mas houve erro ao salvar no histórico local!")
	} else {
		ui.Success("Resposta publicada e salva com sucesso!")
	}
	return nil
}

func (s *CommentService) loadMembersFromCSV(filename string) (map[string]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Warning(fmt.Sprintf("Arquivo de membros '%s' não encontrado.", filename))
			return make(map[string]bool), nil
		}
		return nil, err
	}
	defer file.Close()

	if fileInfo, err := file.Stat(); err == nil {
		if time.Since(fileInfo.ModTime()) > 10*24*time.Hour {
			ui.Warning("Arquivo de membros desatualizado (mais de 10 dias).")
		}
	}

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	members := make(map[string]bool)
	if len(records) > 1 {
		for _, record := range records[1:] {
			if len(record) > 0 {
				channelId := record[1]
				members[channelId] = true
			}
		}
	}
	return members, nil
}
