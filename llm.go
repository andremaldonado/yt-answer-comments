package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

// Models used for analysis and generation. Pick smaller/cheaper for analysis.
const (
	AnalysisModel   = "gemini-2.5-flash-lite" // cheaper/faster model for analysis
	GenerationModel = "gemini-2.5-flash"      // more capable model for text generation
)

// SentimentAnalysis represents the result from the analysis call.
type SentimentAnalysis struct {
	Nota       int    `json:"nota"`
	Sentimento string `json:"sentimento"`
}

// analyzeComment sends the comment to a smaller/cheaper LLM to get nota and sentimento.
func analyzeComment(ctx context.Context, comment string, genaiClient *genai.Client) (SentimentAnalysis, error) {
	prompt := getAnalysisPrompt(comment)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := genaiClient.Models.GenerateContent(
		ctx,
		AnalysisModel,
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return SentimentAnalysis{}, fmt.Errorf("erro ao analisar comentario com Gemini: %w", err)
	}

	raw := resp.Text()
	cleaned := strings.TrimPrefix(raw, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var s SentimentAnalysis
	if err := json.Unmarshal([]byte(cleaned), &s); err != nil {
		return SentimentAnalysis{}, fmt.Errorf("parsing JSON analysis LLM: %w; raw: %s", err, raw)
	}
	return s, nil
}

// suggestAnswer uses the GenerationModel to produce a response text for a given comment.
func suggestAnswer(ctx context.Context, comment string, videoTitle string, videoDescription string, genaiClient *genai.Client) (string, error) {
	prompt := getAnswerPrompt(comment, videoTitle, videoDescription)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := genaiClient.Models.GenerateContent(
		ctx,
		GenerationModel,
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar conte\u00fado com Gemini: %w", err)
	}

	raw := resp.Text()
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, nil
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getAnswerPrompt(comment string, videoTitle string, videoDescription string) string {
	prompt := fmt.Sprintf(`Você é o meu assistente e responde às mensagens que os inscritos do meu canal no Youtube me enviam. É um canal cristão protestante.
	Suas respostas precisam estar relacionadas com o contexto, serem amigáveis e respeitosas.
	Evite adjetivos desnecessários e prefira respostas curtas.
	O comentário que você deve responder é este: "%s"
	O título do vídeo onde o comentário foi feito é: "%s"
	A descrição do vídeo onde o comentário foi feito é: "%s"
	`, comment, videoTitle, videoDescription)
	return prompt
}

// getAnalysisPrompt constructs a short prompt for the analysis model.
func getAnalysisPrompt(comment string) string {
	prompt := fmt.Sprintf(`Você é um classificador de comentários feitos no youtube. 
	
	Para o comentário abaixo, atribua uma nota de 1 a 5 de entendimento do que o comentário quer dizer. Considere que 1 é para um comentário difícil de responder,
	como uma pergunta muito aberta e 5 é para um comentário muito simples, como uma saudação.

	Para o comentário a seguir responda estritamente em JSON com os campos:
	{
		"nota": 0, // inteiro 1..5 onde 5 = muito fácil/resposta óbvia
		"sentimento": "positivo|neutro|negativo" // sentimento geral do comentário
	}

	Exemplo de comentário e resposta esperada:

	Comentário: "Amém""
	Resposta: 
	{
		"nota": 5,
		"sentimento": "positivo"
	}

	Comentário: "Isso é verdade, Deus é maravilhoso!"
	Resposta: 
	{
		"nota": 5,
		"sentimento": "positivo"
	}

	Comentário: "Por que Deus permite tanto sofrimento no mundo?"
	Resposta: 
	{
		"nota": 1,
		"sentimento": "neutro"
	}

	Comentário: "Você não entende nada sobre a Bíblia."
	Resposta: 
	{
		"nota": 3,
		"sentimento": "negativo"
	}

	Comentário: "Falou besteira."
	Resposta:
	{
		"nota": 4,
		"sentimento": "negativo"
	}

	Comentário que deve ser analisado: "%s"
	`, comment)
	return prompt
}
