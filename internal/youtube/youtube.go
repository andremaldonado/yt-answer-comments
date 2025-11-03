package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

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
func GetYoutubeClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getYoutubeTokenFromWeb(config)
		saveYoutubeToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// saveToken to save a token to a file path.
func saveYoutubeToken(path string, token *oauth2.Token) {
	fmt.Printf("Salvando o arquivo de credenciais em: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Não foi possível salvar o token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// getTokenFromWeb uses the OAuth2 config to request a token from the web, then returns the retrieved token.
func getYoutubeTokenFromWeb(config *oauth2.Config) *oauth2.Token {
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
