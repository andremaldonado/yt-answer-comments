package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"answer-comments/internal/models"

	"github.com/openai/openai-go"
)

// getAnalysisModel returns the model for analysis
func getAnalysisModel() string {
	model := os.Getenv("LLM_ANALYSIS_MODEL")
	if model == "" {
		return "deepseek-v4-flash"
	}
	return model
}

// getGenerationModel returns the model for generation
func getGenerationModel() string {
	model := os.Getenv("LLM_GENERATION_MODEL")
	if model == "" {
		return "deepseek-v4-flash"
	}
	return model
}

// buildDateContext builds a string informing the LLM of today's date, the comment's
// publish date, and how many days have passed — so it doesn't assume the comment's
// time-relative references (weekend, holiday, etc.) still apply.
func buildDateContext(commentPublishedAt time.Time) string {
	today := time.Now()
	days := int(today.Sub(commentPublishedAt).Hours() / 24)
	return fmt.Sprintf(
		"\nCONTEXTO TEMPORAL: Hoje é %s. O comentário foi publicado em %s (%d dia(s) atrás). "+
			"Leve essa diferença em conta ao responder — não presuma que ainda é o mesmo dia, fim de semana, feriado ou período mencionado no comentário caso o tempo já tenha passado.\n",
		today.Format("02/01/2006"), commentPublishedAt.Format("02/01/2006"), days,
	)
}

// AnalyzeComment sends the comment to a smaller/cheaper LLM to get nota and sentimento.
func AnalyzeComment(ctx context.Context, comment string, commentPublishedAt time.Time, llmClient openai.Client) (models.SentimentAnalysis, error) {
	prompt := getAnalysisPrompt(comment, commentPublishedAt)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := llmClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(getAnalysisModel()),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return models.SentimentAnalysis{}, fmt.Errorf("erro ao analisar comentario com DeepSeek: %w", err)
	}

	raw := resp.Choices[0].Message.Content
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
func SuggestAnswer(ctx context.Context, isANegativeComment bool, comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string, authorProfile []string, commentPublishedAt time.Time, llmClient openai.Client) (string, error) {

	var prompt string
	if isANegativeComment {
		prompt = getNegativeAnswerPrompt(comment, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, ragContext, authorProfile, commentPublishedAt)
	} else {
		prompt = getPositiveAnswerPrompt(comment, videoTitle, videoDescription, videoTranscript, authorHistory, isMember, ragContext, authorProfile, commentPublishedAt)
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := llmClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(getGenerationModel()),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return "", fmt.Errorf("erro ao gerar conte\u00fado com DeepSeek: %w", err)
	}

	raw := resp.Choices[0].Message.Content
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, nil
}

// buildProfileContext formats known facts about the author into a prompt block.
func buildProfileContext(authorProfile []string) string {
	if len(authorProfile) == 0 {
		return ""
	}
	profileContext := "\nPERFIL CONHECIDO DESTA PESSOA (fatos aprendidos em interações anteriores):\n"
	for _, fact := range authorProfile {
		profileContext += "- " + fact + "\n"
	}
	profileContext += "\nUse esse perfil para personalizar a resposta quando fizer sentido, sem forçar menção a esses fatos.\n"
	return profileContext
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getPositiveAnswerPrompt(comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string, authorProfile []string, commentPublishedAt time.Time) string {
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
	prompt = strings.ReplaceAll(prompt, "{{PROFILE}}", buildProfileContext(authorProfile))
	prompt = strings.ReplaceAll(prompt, "{{DATE_CONTEXT}}", buildDateContext(commentPublishedAt))

	return prompt
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getNegativeAnswerPrompt(comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string, authorProfile []string, commentPublishedAt time.Time) string {
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
	prompt = strings.ReplaceAll(prompt, "{{PROFILE}}", buildProfileContext(authorProfile))
	prompt = strings.ReplaceAll(prompt, "{{DATE_CONTEXT}}", buildDateContext(commentPublishedAt))

	return prompt
}

// getAnalysisPrompt constructs a short prompt for the analysis model.
func getAnalysisPrompt(comment string, commentPublishedAt time.Time) string {
	prompt := os.Getenv("PROMPT_ANALYSIS")
	if prompt == "" {
		return "PROMPT_ANALYSIS not set"
	}
	prompt = strings.ReplaceAll(prompt, "{{COMMENT}}", comment)
	return strings.ReplaceAll(prompt, "{{DATE_CONTEXT}}", buildDateContext(commentPublishedAt))
}

// ExtractProfileFacts analisa o comentário publicado e os fatos já conhecidos sobre o autor,
// retornando apenas os fatos novos e duráveis (vazio se nada de novo for encontrado).
func ExtractProfileFacts(ctx context.Context, comment string, existingFacts []string, llmClient openai.Client) ([]string, error) {
	prompt := getExtractProfilePrompt(comment, existingFacts)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := llmClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(getAnalysisModel()),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao extrair perfil com DeepSeek: %w", err)
	}

	raw := resp.Choices[0].Message.Content
	cleaned := strings.TrimPrefix(raw, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var facts []string
	if err := json.Unmarshal([]byte(cleaned), &facts); err != nil {
		return nil, fmt.Errorf("parsing JSON de extração de perfil: %w; raw: %s", err, raw)
	}
	return facts, nil
}

// getExtractProfilePrompt constructs the prompt used to extract new durable facts about the author.
func getExtractProfilePrompt(comment string, existingFacts []string) string {
	prompt := os.Getenv("PROMPT_EXTRACT_PROFILE")
	if prompt == "" {
		return "PROMPT_EXTRACT_PROFILE not set"
	}

	existingFactsText := "(nenhum)"
	if len(existingFacts) > 0 {
		var b strings.Builder
		for _, fact := range existingFacts {
			b.WriteString("- " + fact + "\n")
		}
		existingFactsText = b.String()
	}

	prompt = strings.ReplaceAll(prompt, "{{COMMENT}}", comment)
	prompt = strings.ReplaceAll(prompt, "{{EXISTING_FACTS}}", existingFactsText)
	return prompt
}
