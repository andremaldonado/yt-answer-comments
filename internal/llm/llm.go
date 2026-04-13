package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"answer-comments/internal/models"

	"google.golang.org/genai"
)

// getAnalysisModel returns the model for analysis
func getAnalysisModel() string {
	model := os.Getenv("LLM_ANALYSIS_MODEL")
	if model == "" {
		return "gemini-2.0-flash-lite"
	}
	return model
}

// getGenerationModel returns the model for generation
func getGenerationModel() string {
	model := os.Getenv("LLM_GENERATION_MODEL")
	if model == "" {
		return "gemini-2.0-flash"
	}
	return model
}

// AnalyzeComment sends the comment to a smaller/cheaper LLM to get nota and sentimento.
func AnalyzeComment(ctx context.Context, comment string, genaiClient *genai.Client) (models.SentimentAnalysis, error) {
	prompt := getAnalysisPrompt(comment)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := genaiClient.Models.GenerateContent(
		ctx,
		getAnalysisModel(),
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return models.SentimentAnalysis{}, fmt.Errorf("erro ao analisar comentario com Gemini: %w", err)
	}

	raw := resp.Text()
	cleaned := strings.TrimPrefix(raw, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var s models.SentimentAnalysis
	if err := json.Unmarshal([]byte(cleaned), &s); err != nil {
		return models.SentimentAnalysis{}, fmt.Errorf("parsing JSON analysis LLM: %w; raw: %s", err, raw)
	}
	return s, nil
}

// suggestAnswer uses the GenerationModel to produce a response text for a given comment.
func SuggestAnswer(ctx context.Context, isANegativeComment bool, comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string, genaiClient *genai.Client) (string, error) {

	var prompt string
	if isANegativeComment {
		prompt = getNegativeAnswerPrompt(comment, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, ragContext)
	} else {
		prompt = getPositiveAnswerPrompt(comment, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, ragContext)
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := genaiClient.Models.GenerateContent(
		ctx,
		getGenerationModel(),
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
func getPositiveAnswerPrompt(comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string) string {
	prompt := os.Getenv("PROMPT_POSITIVE_ANSWER")
	if prompt == "" {
		// Fallback removed for brevity in this tool call, but ideally keep a minimal default or just log/error
		return "PROMPT_POSITIVE_ANSWER not set"
	}

	var historyContext string
	if len(authorHistory) > 0 {
		historyContext = "\nHistórico de interações anteriores com esta pessoa:\n"
		for i, h := range authorHistory {
			historyContext += fmt.Sprintf("Comentário anterior %d: %s\nResposta dada: %s\n",
				i+1, h.CommentText, h.Response)
		}
	}

	var consistencyContext string
	if len(ragContext) > 0 {
		consistencyContext = "\nINSTRUÇÃO DE CONSISTÊNCIA: No passado, respondi a comentários similares (mesmo tema e sentimento) da seguinte forma:\n"
		for _, c := range ragContext {
			consistencyContext += c + "\n"
		}
		consistencyContext += "\nUse essas respostas como base de tom e doutrina para gerar a nova resposta para o comentário atual.\n"
	}

	var transcriptContext string
	if videoTranscript != "" {
		transcriptContext = fmt.Sprintf("\nTRANSCRIÇÃO DO VÍDEO: Use esta transcrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoTranscript)
	}

	var memberNotice string
	if isMember {
		memberNotice = "\nNote que este usuário é membro do canal, então seja um pouco mais caloroso e agradecido na resposta.\n"
	}

	prompt = strings.ReplaceAll(prompt, "{{COMMENT}}", comment)
	prompt = strings.ReplaceAll(prompt, "{{TITLE}}", videoTitle)
	prompt = strings.ReplaceAll(prompt, "{{DESCRIPTION}}", videoDescription)
	prompt = strings.ReplaceAll(prompt, "{{TRANSCRIPT}}", transcriptContext)
	prompt = strings.ReplaceAll(prompt, "{{HISTORY}}", historyContext)
	prompt = strings.ReplaceAll(prompt, "{{CONSISTENCY}}", consistencyContext)
	prompt = strings.ReplaceAll(prompt, "{{MEMBER_NOTICE}}", memberNotice)

	return prompt
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getNegativeAnswerPrompt(comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string) string {
	prompt := os.Getenv("PROMPT_NEGATIVE_ANSWER")
	if prompt == "" {
		return "PROMPT_NEGATIVE_ANSWER not set"
	}

	var historyContext string
	if len(authorHistory) > 0 {
		historyContext = "\nHistórico de interações anteriores com esta pessoa:\n"
		for i, h := range authorHistory {
			historyContext += fmt.Sprintf("Comentário anterior %d: %s\nResposta dada: %s\n",
				i+1, h.CommentText, h.Response)
		}
	}

	var consistencyContext string
	if len(ragContext) > 0 {
		consistencyContext = "\nINSTRUÇÃO DE CONSISTÊNCIA: No passado, respondi a comentários similares (mesmo tema e sentimento) da seguinte forma:\n"
		for _, c := range ragContext {
			consistencyContext += c + "\n"
		}
		consistencyContext += "\nUse essas respostas como base de tom e doutrina para gerar a nova resposta para o comentário atual.\n"
	}

	var transcriptContext string
	if videoTranscript != "" {
		transcriptContext = fmt.Sprintf("\nTRANSCRIÇÃO DO VÍDEO: Use esta transcrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoTranscript)
	}

	var descriptionContext string
	if videoDescription != "" {
		descriptionContext = fmt.Sprintf("\nDESCRIÇÃO DO VÍDEO: Use esta descrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoDescription)
	}

	var memberNotice string
	if isMember {
		memberNotice = "\nNote que este usuário é membro do canal, considere isso ao dar a resposta, agradecendo o apoio.\n"
	}

	prompt = strings.ReplaceAll(prompt, "{{COMMENT}}", comment)
	prompt = strings.ReplaceAll(prompt, "{{TITLE}}", videoTitle)
	prompt = strings.ReplaceAll(prompt, "{{DESCRIPTION}}", descriptionContext)
	prompt = strings.ReplaceAll(prompt, "{{TRANSCRIPT}}", transcriptContext)
	prompt = strings.ReplaceAll(prompt, "{{HISTORY}}", historyContext)
	prompt = strings.ReplaceAll(prompt, "{{CONSISTENCY}}", consistencyContext)
	prompt = strings.ReplaceAll(prompt, "{{MEMBER_NOTICE}}", memberNotice)

	return prompt
}

// getAnalysisPrompt constructs a short prompt for the analysis model.
func getAnalysisPrompt(comment string) string {
	prompt := os.Getenv("PROMPT_ANALYSIS")
	if prompt == "" {
		return "PROMPT_ANALYSIS not set"
	}
	return strings.ReplaceAll(prompt, "{{COMMENT}}", comment)
}
