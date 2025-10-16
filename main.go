package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	b, err := os.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Não foi possível ler o arquivo client_secret.json: %v", err)
	}

	// Load OAuth2 config for YouTube
	config, err := google.ConfigFromJSON(b, youtube.YoutubeForceSslScope, youtube.YoutubeChannelMembershipsCreatorScope)
	if err != nil {
		log.Fatalf("Não foi possível analisar o arquivo de segredo do cliente: %v", err)
	}
	client := getYoutubeClient(config)

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

				isMember := membersMap[comment.Snippet.AuthorChannelId.Value]
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

				// Show comment details
				fmt.Println("")
				fmt.Println("------------------------------------------------------------------")
				fmt.Println("             Novo comentário não respondido encontrado            ")
				fmt.Println("------------------------------------------------------------------")
				brTime := commentPublishedAt.In(time.FixedZone("BRT", -3*60*60))
				fmt.Printf("Título do vídeo: %s\n", videoTitle)
				fmt.Printf("%sAutor: %s (Publicado em: %s)\n", authorPrefix, comment.Snippet.AuthorDisplayName, brTime.Format("02/01/2006 às 15:04"))
				fmt.Printf("Comentário: %s\n", comment.Snippet.TextDisplay)

				// Suggest answer using Gemini
				fmt.Println("")
				fmt.Println("Gerando sugestão de resposta...")
				fmt.Println("")
				suggestedAnswer := LLMSuggestion{}
				suggestedAnswer, err = suggestAnswer(ctx, comment.Snippet.TextOriginal, videoTitle, videoDescription, geminiClient)

				if suggestedAnswer.Resposta == "" || err != nil {
					fmt.Println("⚠️ Não foi possível gerar uma sugestão de resposta para este comentário.")
					fmt.Println("🚫 Resposta não publicada. Seguindo para o próximo comentário.")
					fmt.Println("Error:", err)
					fmt.Println("")
					continue // Jump to the next comment
				}

				// Show suggested answer and note
				answer := strings.TrimSpace(suggestedAnswer.Resposta)
				fmt.Printf("Sugestão de resposta: %s\n\n", answer)
				fmt.Printf("Nota de entendimento atribuída: %d\n", suggestedAnswer.Nota)

				// Check the answer with the user
				fmt.Printf("\nDeseja publicar esta resposta? (S/N/E/Q para sair): ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToUpper(input))

				switch input {
				case "S":
					err := publishComment(service, comment.Id, answer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						fmt.Println("Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						fmt.Println("✅ Resposta publicada com sucesso!")
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
					err := publishComment(service, comment.Id, editedAnswer)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						fmt.Println("Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						fmt.Println("✅ Resposta editada publicada com sucesso!")
					}
				case "Q":
					fmt.Println("Encerrando a aplicação.")
					return
				default:
					fmt.Println("🚫 Resposta não publicada. Seguindo para o próximo comentário.")
				}

				// Log comment and suggestion to a file
				addToLog(comment, brTime, suggestedAnswer, answer)

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

// addToLog appends the comment and its suggested answer to a CSV log file.
func addToLog(comment *youtube.Comment, brTime time.Time, suggestedAnswer LLMSuggestion, answer string) {
	logFile, err := os.OpenFile("comment_log.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Não foi possível abrir o arquivo de log: %v", err)
	} else {
		defer logFile.Close()
		logger := log.New(logFile, "", log.LstdFlags)
		logger.Printf("%s;%s;%s;%d;%s\n",
			strings.ReplaceAll(comment.Snippet.AuthorDisplayName, ";", ","),
			brTime.Format("02/01/2006 às 15:04"),
			strings.ReplaceAll(strings.ReplaceAll(comment.Snippet.TextOriginal, ";", ","), "\n", " "),
			suggestedAnswer.Nota,
			strings.ReplaceAll(strings.ReplaceAll(answer, ";", ","), "\n", " "),
		)
	}
}

// loadMembersFromCSV lê um arquivo CSV com a lista de membros e retorna um mapa de Channel IDs.
// O mapa é usado para uma verificação rápida (O(1) em média).
func loadMembersFromCSV(filename string) (map[string]bool, error) {
	// Abre o arquivo CSV
	file, err := os.Open(filename)
	if err != nil {
		// Retorna um mapa vazio se o arquivo não existir, para que o programa não quebre.
		if os.IsNotExist(err) {
			fmt.Printf("Aviso: Arquivo de membros '%s' não encontrado. A identificação de membros estará desativada.\n", filename)
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("erro ao abrir o arquivo de membros: %w", err)
	}
	defer file.Close()

	// Verifica se o arquivo é mais antigo que 10 dias
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter informações do arquivo de membros: %w", err)
	}
	if time.Since(fileInfo.ModTime()) > 10*24*time.Hour {
		fmt.Printf("ATENÇÃO: O arquivo de membros '%s' está desatualizado (última modificação em %s). Considere atualizá-lo.\n", filename, fileInfo.ModTime().Format("02/01/2006"))
	}

	// Cria um leitor de CSV
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler o arquivo de membros: %w", err)
	}

	members := make(map[string]bool)
	if len(records) > 1 { // Pula o cabeçalho (linha 0)
		// Assumindo que o ID do Canal está na primeira coluna (índice 0)
		// IMPORTANTE: Verifique seu arquivo CSV para confirmar a coluna correta!
		for _, record := range records[1:] {
			if len(record) > 0 {
				channelId := record[1]
				members[channelId] = true
			}
		}
	}

	return members, nil
}
