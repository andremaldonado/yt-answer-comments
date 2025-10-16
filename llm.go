package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

// LLMSuggestion is the structure for the LLM's suggested answer and understanding score.
type LLMSuggestion struct {
	Nota     int    `json:"nota"`
	Resposta string `json:"resposta"`
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
