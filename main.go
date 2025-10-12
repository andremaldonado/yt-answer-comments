package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"google.golang.org/genai"
)

// LLMSuggestion is the structure for the LLM's suggested answer and understanding score.
type LLMSuggestion struct {
	Nota     int    `json:"nota"`
	Resposta string `json:"resposta"`
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// getTokenFromWeb uses the OAuth2 config to request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Acesse esta URL no seu navegador para autorizar o aplicativo: \n%v\n", authURL)
	fmt.Printf("\nApós autorizar, cole o código de autorização aqui e pressione Enter: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Não foi possível ler o código de autorização: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Não foi possível obter o token a partir do código: %v", err)
	}
	return tok
}

// tokenFromFile reads a token from a file path.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken to save a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Salvando o arquivo de credenciais em: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Não foi possível salvar o token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// publishComment to publish a reply to a comment on YouTube.
func publishComment(service *youtube.Service, videoId, parentCommentId, text string) error {
	comment := &youtube.Comment{
		Snippet: &youtube.CommentSnippet{
			ParentId:     parentCommentId,
			TextOriginal: text,
		},
	}
	// A API Comments.Insert exige o part="snippet" e o id do tópico (parentCommentId)
	// para que o comentário seja uma resposta.
	// O canal que responde é o autenticado.
	call := service.Comments.Insert([]string{"snippet"}, comment)
	_, err := call.Do()
	if err != nil {
		return fmt.Errorf("erro ao publicar resposta: %v", err)
	}
	return nil
}

func main() {
	ctx := context.Background()

	b, err := os.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Não foi possível ler o arquivo client_secret.json: %v", err)
	}

	config, err := google.ConfigFromJSON(b, youtube.YoutubeForceSslScope)
	if err != nil {
		log.Fatalf("Não foi possível analisar o arquivo de segredo do cliente: %v", err)
	}

	client := getClient(config)

	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Erro ao criar o serviço do YouTube: %v", err)
	}

	channelResponse, err := service.Channels.List([]string{"id"}).Mine(true).Do()
	if err != nil {
		log.Fatalf("Erro ao obter o ID do canal: %v", err)
	}
	if len(channelResponse.Items) == 0 {
		log.Fatalf("Não foi possível encontrar o ID do canal do usuário autenticado.")
	}
	myChannelId := channelResponse.Items[0].Id
	fmt.Printf("Autenticado com sucesso! ID do seu canal: %s\n\n", myChannelId)

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
				fmt.Printf("Autor: %s (Publicado em: %s)\n", comment.Snippet.AuthorDisplayName, brTime.Format("02/01/2006 às 15:04"))
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
				fmt.Printf("Sugestão de resposta: %s\n\n", suggestedAnswer.Resposta)
				fmt.Printf("Nota de entendimento atribuída: %d\n", suggestedAnswer.Nota)

				if suggestedAnswer.Nota < 5 {
					fmt.Println("⚠️ A nota de entendimento é menor que 5. Pulando comentário.")
					continue // Jump to the next comment
				}

				// Log comment and suggestion to a file
				logFile, err := os.OpenFile("comment_log.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					log.Printf("Não foi possível abrir o arquivo de log: %v", err)
				} else {
					defer logFile.Close()
					logger := log.New(logFile, "", log.LstdFlags)
					logger.Printf("%s;%s;%s;%d;%s\n",
						strings.Replace(comment.Snippet.AuthorDisplayName, ";", ",", -1),
						brTime.Format("02/01/2006 às 15:04"),
						strings.Replace(comment.Snippet.TextOriginal, ";", ",", -1),
						suggestedAnswer.Nota,
						strings.Replace(suggestedAnswer.Resposta, ";", ",", -1),
					)
				}

				// Check the answer with the user
				fmt.Printf("\nDeseja publicar esta resposta? (S/N/Q para sair): ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToUpper(input))

				switch input {
				case "S":
					err := publishComment(service, comment.Snippet.VideoId, comment.Id, suggestedAnswer.Resposta)
					if err != nil {
						log.Printf("Falha ao publicar resposta: %v", err)
						fmt.Println("Erro ao publicar a resposta. Tente novamente mais tarde.")
					} else {
						fmt.Println("✅ Resposta publicada com sucesso!")
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

// suggestAnswer uses the Gemini model to generate a suggested answer for a given comment.
func suggestAnswer(ctx context.Context, comment string, videoTitle string, videoDescription string, genaiClient *genai.Client) (LLMSuggestion, error) {
	prompt := getAnswerPrompt(comment, videoTitle, videoDescription)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := genaiClient.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return LLMSuggestion{}, fmt.Errorf("erro ao gerar conteúdo com Gemini: %w", err)
	}

	raw := resp.Text()
	cleaned := strings.TrimPrefix(raw, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var s LLMSuggestion
	if err := json.Unmarshal([]byte(cleaned), &s); err != nil {
		return LLMSuggestion{}, fmt.Errorf("parsing JSON LLM: %w; raw: %s", err, raw)
	}
	return s, nil
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getAnswerPrompt(comment string, videoTitle string, videoDescription string) string {
	prompt := fmt.Sprintf(`Você é o meu assistente e responde às mensagens que os inscritos do meu canal no Youtube me enviam. É um canal cristão protestante.
	Suas respostas precisam estar relacionadas com o contexto, serem amigáveis e respeitosas.
	Evite adjetivos desnecessários e prefira respostas curtas.
	Para cada comentário, atribua uma nota de 1 a 5 de entendimento do que o comentário quer dizer. Considere que 1 é para um comentário difícil de responder,
	como uma pergunta muito aberta e 5 é para um comentário muito simples, como uma saudação.
	Sua resposta deve ser sempre no seguinte formato, sem nada além disso, nem mesmo uma marcação de json:
	{
		"nota": 0,
		"resposta": "Sua resposta aqui"
	}
	O comentário que você deve responder é este: "%s"
	O título do vídeo onde o comentário foi feito é: "%s"
	A descrição do vídeo onde o comentário foi feito é: "%s"
	`, comment, videoTitle, videoDescription)
	return prompt
}
