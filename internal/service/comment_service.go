package service

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"answer-comments/internal/app"
	"answer-comments/internal/database"
	"answer-comments/internal/llm"
	"answer-comments/internal/models"
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
	fmt.Printf("✅ Carregados %d membros a partir do arquivo.\n\n", len(membersMap))

	reader := bufio.NewReader(os.Stdin)
	var pageToken string

	fmt.Print("-> Pressione Enter para iniciar a verificação de novos comentários não respondidos...")
	_, _ = reader.ReadString('\n')

	for {
		fmt.Println("")
		fmt.Println("------------------------------------------------------------------")
		fmt.Println("Buscando novos comentários não respondidos...")
		fmt.Println("------------------------------------------------------------------")

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
				if err := s.handleUnansweredComment(ctx, comment, commentPublishedAt, membersMap, opts, reader); err != nil {
					log.Printf("Erro ao processar comentário %s: %v", comment.Id, err)
					time.Sleep(15 * time.Second)
				}
			}
		}

		if !foundUnanswered {
			if pageToken == "" {
				fmt.Println("\nNão há mais comentários não respondidos em todas as páginas disponíveis.")
				return nil
			} else {
				fmt.Println("\nNão há mais comentários não respondidos neste lote.")
				fmt.Printf("Pressione Enter para buscar o próximo lote de comentários, ou digite 'Q' para sair: ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToUpper(input))
				if input == "Q" {
					return nil
				}
			}
		}
	}
}

func (s *CommentService) handleUnansweredComment(ctx context.Context, comment *youtube.Comment, publishedAt time.Time, membersMap map[string]bool, opts AnswerOptions, reader *bufio.Reader) error {
	isMember := membersMap["https://www.youtube.com/channel/"+comment.Snippet.AuthorChannelId.Value]
	authorPrefix := ""
	if isMember {
		authorPrefix = "⭐ MEMBRO ⭐ "
	}

	videoCall := s.App.YTService.Videos.List([]string{"snippet"}).Id(comment.Snippet.VideoId)
	videoResp, err := videoCall.Do()
	videoTitle := "[Não foi possível obter o título]"
	videoDescription := "[Não foi possível obter a descrição]"
	if err == nil && len(videoResp.Items) > 0 {
		videoTitle = videoResp.Items[0].Snippet.Title
		videoDescription = videoResp.Items[0].Snippet.Description
	}

	fmt.Print("\033[H\033[2J")
	fmt.Println("------------------------------------------------------------------")
	fmt.Println("             Novo comentário não respondido encontrado            ")
	fmt.Println("------------------------------------------------------------------")
	fmt.Println("")

	brTime := publishedAt.In(time.FixedZone("BRT", -3*60*60))
	fmt.Println("# Detalhes do comentário")
	fmt.Printf("Título do vídeo: %s\n", videoTitle)
	fmt.Printf("%sAutor: %s (Publicado em: %s)\n", authorPrefix, comment.Snippet.AuthorDisplayName, brTime.Format("02/01/2006 às 15:04"))
	fmt.Printf("Comentário: %s\n\n", comment.Snippet.TextDisplay)

	fmt.Println("# Análise do comentário")
	sentiment, err := llm.AnalyzeComment(ctx, comment.Snippet.TextOriginal, s.App.GeminiClient)
	if err != nil {
		return fmt.Errorf("erro na análise de sentimento: %w", err)
	}
	fmt.Printf("Análise de sentimento: %s\n", sentiment.Sentimento)
	fmt.Printf("Nota de entendimento: %d\n", sentiment.Nota)
	fmt.Printf("Tema: %s\n\n", sentiment.Tema)

	var answer, suggestedAnswer, input string
	if opts.ManualMode {
		input = "E"
	}

	pastAnswers, err := database.GetPreviousAnswersByContext(sentiment.Tema, sentiment.Sentimento, 5)
	if err != nil {
		log.Printf("⚠️ Erro ao buscar histórico de RAG: %v", err)
	}

	authorHistory, err := database.GetLastComments(comment.Snippet.AuthorDisplayName, 10)
	if err != nil {
		log.Printf("⚠️ Erro ao buscar histórico de comentários: %v", err)
	}

	shouldSuggestAnswer := !opts.ManualMode && sentiment.Sentimento != "negativo" && sentiment.Nota >= 3
	if shouldSuggestAnswer {
		var videoTranscript string
		if opts.TranscriptionMode && sentiment.Tema != "Saudação/Agradecimento" {
			fmt.Println("# Transcrição do vídeo")
			fmt.Printf("Buscando transcrição do vídeo...\n")
			videoTranscript, err = yt.GetVideoTranscription(ctx, s.App.YTService, comment.Snippet.VideoId)
			if err != nil {
				log.Printf("⚠️ Não foi possível obter a transcrição: %v", err)
			} else {
				fmt.Printf("✅ Transcrição obtida com sucesso (%d caracteres)\n\n", len(videoTranscript))
			}
		}

		fmt.Println("# RAG")
		if len(authorHistory) > 0 {
			fmt.Printf("✅ %d mensagens encontradas no histórico de interações anteriores com esta pessoa.\n", len(authorHistory))
		}
		if len(pastAnswers) > 0 {
			fmt.Printf("✅ %d respostas similares encontradas no histórico.\n", len(pastAnswers))
		}

		fmt.Println("\n# Sugestão de resposta")
		suggestedAnswer, err = llm.SuggestAnswer(ctx, sentiment.Sentimento == "negativo", comment.Snippet.TextOriginal, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, pastAnswers, s.App.GeminiClient)
		if err != nil {
			return fmt.Errorf("erro ao sugerir resposta: %w", err)
		}

		if suggestedAnswer == "" {
			fmt.Println("⚠️ Não foi possível gerar uma sugestão de resposta.")
			return nil
		}

		answer = strings.TrimSpace(suggestedAnswer)
		fmt.Printf("%s\n", answer)

		if sentiment.Sentimento == "positivo" && sentiment.Nota >= 4 && opts.AutoAnswerMode {
			input = "S"
			time.Sleep(2 * time.Second)
			fmt.Println("✅ Resposta sugerida será publicada automaticamente.")
			time.Sleep(3 * time.Second)
		}

		if input == "" {
			fmt.Printf("\nDeseja publicar esta resposta? (S/N/E/Q para sair): ")
			input, _ = reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToUpper(input))
		}
	}

	if suggestedAnswer == "" && input == "" {
		fmt.Println("⚠️ Optei por não gerar uma resposta automática.")
		input = "E"
	}

	switch input {
	case "S":
		return s.publishAndSave(comment, &sentiment, answer, false)
	case "E":
		fmt.Print("Digite a resposta que deseja publicar:\n> ")
		editedAnswer, _ := reader.ReadString('\n')
		editedAnswer = strings.TrimSpace(editedAnswer)
		if editedAnswer == "" {
			fmt.Println("🚫 Resposta vazia.")
			return nil
		}
		return s.publishAndSave(comment, &sentiment, editedAnswer, true)
	case "Q":
		os.Exit(0)
	default:
		fmt.Println("🚫 Resposta não publicada.")
	}
	return nil
}

func (s *CommentService) publishAndSave(comment *youtube.Comment, sentiment *models.SentimentAnalysis, answer string, userAnswered bool) error {
	err := yt.PublishComment(s.App.YTService, comment.Id, answer)
	if err != nil {
		return fmt.Errorf("falha ao publicar resposta: %w", err)
	}

	if err := database.SaveComment(comment, sentiment.Sentimento, sentiment.Nota, sentiment.Tema, answer, userAnswered); err != nil {
		log.Printf("⚠️ Erro ao salvar no banco: %v", err)
		fmt.Println("✅ Resposta publicada, mas houve erro ao salvar no histórico local!")
	} else {
		fmt.Println("✅ Resposta publicada e salva com sucesso!")
	}
	return nil
}

func (s *CommentService) loadMembersFromCSV(filename string) (map[string]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Aviso: Arquivo de membros '%s' não encontrado.\n", filename)
			return make(map[string]bool), nil
		}
		return nil, err
	}
	defer file.Close()

	if fileInfo, err := file.Stat(); err == nil {
		if time.Since(fileInfo.ModTime()) > 10*24*time.Hour {
			fmt.Printf("ATENÇÃO: Arquivo de membros desatualizado.\n")
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
