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

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"google.golang.org/genai"
)

func main() {
	// Parse command line flags
	manualMode := flag.Bool("manual", false, "Modo manual: pula a sugestão da LLM e força edição manual de todas as respostas")
	flag.BoolVar(manualMode, "m", false, "Atalho para --manual")
	autoAnswerMode := flag.Bool("auto", false, "Modo auto-resposta: todas as respostas sugeridas e com alto nível de confiança pela LLM serão publicadas automaticamente sem confirmação")
	flag.BoolVar(autoAnswerMode, "a", false, "Atalho para --auto")
	flag.Parse()

	// Flag - Manual mode
	if *manualMode {
		fmt.Println("⚠️ Modo manual ativado: todas as respostas deverão ser editadas manualmente. ⚠️")
	}

	// Flag - Auto-answer mode
	if *autoAnswerMode {
		fmt.Println("⚠️ Modo auto-resposta ativado: todas as respostas sugeridas e com alto nível de confiança serão publicadas automaticamente sem confirmação. ⚠️")
	}

	ctx := context.Background()

	// Initialize database
	if err := database.InitDB(); err != nil {
		log.Fatalf("Erro ao inicializar o banco de dados: %v", err)
	}
	defer database.CloseDB()

	b, err := os.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Não foi possível ler o arquivo client_secret.json: %v", err)
	}

	// Load OAuth2 config for YouTube
	config, err := google.ConfigFromJSON(b, yt.YoutubeForceSslScope, yt.YoutubeChannelMembershipsCreatorScope)
	if err != nil {
		log.Fatalf("Não foi possível analisar o arquivo de segredo do cliente: %v", err)
	}
	client := yt.GetYoutubeClient(config)

	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Erro ao criar o serviço do YouTube: %v", err)
	}

	// Get the authenticated user's channel ID
	channelResponse, err := service.Channels.List([]string{"id"}).Mine(true).Do()
	if err != nil {
		log.Fatalf("Erro ao obter o ID do canal: %v", err)
	}
	if len(channelResponse.Items) == 0 {
		log.Fatalf("Não foi possível encontrar o ID do canal do usuário autenticado.")
	}
	myChannelId := channelResponse.Items[0].Id
	fmt.Printf("Autenticado com sucesso! ID do seu canal: %s\n\n", myChannelId)

	// load members from CSV
	membersMap, err := loadMembersFromCSV("members.csv")
	if err != nil {
		log.Fatalf("Não foi possível carregar a lista de membros: %v", err)
	}
	fmt.Printf("Carregados %d membros a partir do arquivo.\n", len(membersMap))

	// Initialize Gemini client
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("A variável de ambiente GEMINI_API_KEY não está configurada.")
	}
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  geminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Prepare to read user input
	reader := bufio.NewReader(os.Stdin)
	var pageToken string

	// Ask for user confirmation before starting
	fmt.Print("Pressione Enter para iniciar a verificação de novos comentários não respondidos...")
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
			log.Fatalf("Erro ao buscar os comentários: %v", err)
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
				fmt.Println("# Análise do comentário")
				sentiment, err := llm.AnalyzeComment(ctx, comment.Snippet.TextOriginal, geminiClient)
				if err != nil {
					fmt.Println("⚠️ Não foi possível analisar o sentimento deste comentário, pulando para o próximo.")
					fmt.Println("Error:", err)
					os.Exit(1)
					continue // Jump to the next comment
				}
				fmt.Printf("Análise de sentimento: %s\n", sentiment.Sentimento)
				fmt.Printf("Nota de entendimento: %d\n", sentiment.Nota)
				fmt.Printf("Tema: %s\n\n", sentiment.Tema)

				var answer, suggestedAnswer, input string

				// In manual mode, do not generate suggested answer
				if *manualMode {
					input = "E" // Forces manual edit mode
					suggestedAnswer = ""
				}

				// Buscar exemplos anteriores para RAG
				pastAnswers, err := database.GetPreviousAnswersByContext(sentiment.Tema, sentiment.Sentimento, 5)
				if err != nil {
					log.Printf("Erro ao buscar histórico de RAG: %v", err)
					// Não pare a execução, apenas continue sem o contexto
				}

				shouldSuggestAnswer := !*manualMode && sentiment.Sentimento != "negativo" && sentiment.Nota >= 3
				if shouldSuggestAnswer {
					// Search comment history from this author
					authorHistory, err := database.GetLastComments(comment.Snippet.AuthorDisplayName, 3)
					if err != nil {
						log.Printf("⚠️ Erro ao buscar histórico de comentários: %v", err)
						authorHistory = nil // continues without history
					}

					// If there is history, show to user
					if len(authorHistory) > 0 {
						fmt.Println("# RAG")
						fmt.Println("Histórico de interações anteriores com esta pessoa:")
						for i, h := range authorHistory {
							fmt.Printf("\nComentário anterior %d (%s):\n%s\n", i+1, h.CreatedAt.Format("02/01/2006"), h.CommentText)
							if h.Response != "" {
								fmt.Printf("Resposta dada: %s\n", h.Response)
							}
						}
						fmt.Println("")
					}

					// Suggest answer using Gemini
					fmt.Println("# Sugestão de resposta")
					suggestedAnswer, err = llm.SuggestAnswer(ctx, sentiment.Sentimento == "negativo", comment.Snippet.TextOriginal, videoTitle, videoDescription, authorHistory, isMember, pastAnswers, geminiClient)

					if suggestedAnswer == "" || err != nil {
						fmt.Println("⚠️ Não foi possível gerar uma sugestão de resposta para este comentário.")
						fmt.Println("🚫 Resposta não publicada. Seguindo para o próximo comentário.")
						fmt.Println("Error:", err)
						fmt.Println("")
						continue // Jump to the next comment
					}

					// Show suggested answer and note
					answer = strings.TrimSpace(suggestedAnswer)
					fmt.Printf("%s\n\n", answer)

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
