package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/youtube/v3"
)

const (
	// YoutubeForceSslScope is the scope required for SSL access
	YoutubeForceSslScope = youtube.YoutubeForceSslScope
	// YoutubeChannelMembershipsCreatorScope is the scope required for channel memberships
	YoutubeChannelMembershipsCreatorScope = youtube.YoutubeChannelMembershipsCreatorScope
)

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

// GetYoutubeClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func GetYoutubeClient(config *oauth2.Config) (*http.Client, error) {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = getYoutubeTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("erro ao obter token da web: %w", err)
		}
		if err := saveYoutubeToken(tokFile, tok); err != nil {
			return nil, fmt.Errorf("erro ao salvar token: %w", err)
		}
	}
	return config.Client(context.Background(), tok), nil
}

// saveToken to save a token to a file path.
func saveYoutubeToken(path string, token *oauth2.Token) error {
	fmt.Printf("Salvando o arquivo de credenciais em: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("não foi possível salvar o token: %w", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// getTokenFromWeb uses the OAuth2 config to request a token from the web, then returns the retrieved token.
func getYoutubeTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Acesse esta URL no seu navegador para autorizar o aplicativo: \n%v\n", authURL)
	fmt.Printf("\nApós autorizar, cole o código de autorização aqui e pressione Enter: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("não foi possível ler o código de autorização: %w", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("não foi possível obter o token a partir do código: %w", err)
	}
	return tok, nil
}

// PublishComment posts a reply to a YouTube comment
func PublishComment(service *youtube.Service, parentId string, text string) error {
	comment := &youtube.Comment{
		Snippet: &youtube.CommentSnippet{
			ParentId:     parentId,
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

// GetVideoTranscription fetches the automatic caption/transcript for a video if available
func GetVideoTranscription(ctx context.Context, service *youtube.Service, videoId string) (string, error) {
	// List all available captions for the video
	captionsListCall := service.Captions.List([]string{"snippet"}, videoId)
	captionsListResponse, err := captionsListCall.Do()
	if err != nil {
		return "", fmt.Errorf("erro ao listar legendas: %w", err)
	}

	// Find an automatic caption (prefer Portuguese, then English, then any)
	var captionId string
	var captionPriority int // 3 = pt, 2 = en, 1 = other

	for _, caption := range captionsListResponse.Items {
		if caption.Snippet.TrackKind != "asr" { // asr = automatic speech recognition
			continue
		}

		priority := 1
		if caption.Snippet.Language == "pt" || caption.Snippet.Language == "pt-BR" {
			priority = 3
		} else if caption.Snippet.Language == "en" {
			priority = 2
		}

		if priority > captionPriority {
			captionId = caption.Id
			captionPriority = priority
		}
	}

	if captionId == "" {
		return "", fmt.Errorf("nenhuma legenda automática encontrada para este vídeo")
	}

	// Download the caption
	captionDownloadCall := service.Captions.Download(captionId).Tfmt("srt")
	resp, err := captionDownloadCall.Download()
	if err != nil {
		return "", fmt.Errorf("erro ao baixar legenda: %w", err)
	}
	defer resp.Body.Close()

	// Read the transcript
	var transcript strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			transcript.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	return cleanTranscript(transcript.String()), nil
}

// cleanTranscript removes SRT formatting and returns only the text
func cleanTranscript(srt string) string {
	lines := strings.Split(srt, "\n")
	var cleanText []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines, numbers, and timestamps
		if line == "" || isNumber(line) || strings.Contains(line, "-->") {
			continue
		}
		cleanText = append(cleanText, line)
	}

	return strings.Join(cleanText, " ")
}

// isNumber checks if a string is a number
func isNumber(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
