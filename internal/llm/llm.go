package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"answer-comments/internal/models"

	"google.golang.org/genai"
)

// Models used for analysis and generation. Pick smaller/cheaper for analysis.
const (
	AnalysisModel   = "gemini-2.5-flash-lite" // cheaper/faster model for analysis
	GenerationModel = "gemini-2.5-flash"      // more capable model for text generation
)

// AnalyzeComment sends the comment to a smaller/cheaper LLM to get nota and sentimento.
func AnalyzeComment(ctx context.Context, comment string, genaiClient *genai.Client) (models.SentimentAnalysis, error) {
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
		GenerationModel,
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.3),
		},
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
	var historyContext string
	if len(authorHistory) > 0 {
		historyContext = "\nHistórico de interações anteriores com esta pessoa:\n"
		for i, h := range authorHistory {
			historyContext += fmt.Sprintf("Comentário anterior %d: %s\nResposta dada: %s\n",
				i+1, h.CommentText, h.Response)
		}
	}

	prompt := fmt.Sprintf(`Você é o meu assistente e responde às mensagens que os inscritos do meu canal no Youtube me enviam. É um canal cristão protestante.
	Suas respostas precisam estar relacionadas com o contexto, serem amigáveis e respeitosas.
	Evite adjetivos desnecessários e prefira respostas curtas, sem repetir o que a pessoa falou de maneira desnecessária.
	Use sempre linguagem neutra e impessoal, sem tentar inferir se a pessoa é homem ou mulher. Nunca use termos com flexão de gênero como 'obrigado', 'obrigada', 'abençoado', 'abençoada', 'fico feliz que tenha gostado' (masc/fem). Prefira alternativas neutras como 'Agradeço pelo comentário', 'Que bom que gostou', 'Deus abençoe'.
	O comentário que você deve responder é este: "%s"
	O título do vídeo onde o comentário foi feito é: "%s"
	A descrição do vídeo onde o comentário foi feito é: "%s"
	`, comment, videoTitle, videoDescription)

	if videoTranscript != "" {
		prompt += fmt.Sprintf("\nTRANSCRIÇÃO DO VÍDEO: Use esta transcrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoTranscript)
	}

	if historyContext != "" {
		prompt = fmt.Sprintf(`%s
		O histórico de interações anteriores com esta pessoa é o seguinte:
		%s
		Use esse histórico para evitar repetir respostas ou entrar em discussões.
		Também leve em conta o histórico para ajustar o tom da resposta.
		`, prompt, historyContext)
	}

	var consistencyContext string
	if len(ragContext) > 0 {
		consistencyContext = "\nINSTRUÇÃO DE CONSISTÊNCIA: No passado, respondi a comentários similares (mesmo tema e sentimento) da seguinte forma:\n"
		for _, c := range ragContext {
			consistencyContext += c + "\n"
		}
		consistencyContext += "\nUse essas respostas como base de tom e doutrina para gerar a nova resposta para o comentário atual.\n"

		prompt += consistencyContext
	}

	if isMember {
		prompt += "\nNote que este usuário é membro do canal, então seja um pouco mais caloroso e agradecido na resposta.\n"
	}

	return prompt
}

// getAnswerPrompt constructs the prompt for the LLM based on the comment and video context.
func getNegativeAnswerPrompt(comment string, videoTitle string, videoDescription string, videoTranscript string, authorHistory []models.Comment, isMember bool, ragContext []string) string {
	var historyContext string
	if len(authorHistory) > 0 {
		historyContext = "\nHistórico de interações anteriores com esta pessoa:\n"
		for i, h := range authorHistory {
			historyContext += fmt.Sprintf("Comentário anterior %d: %s\nResposta dada: %s\n",
				i+1, h.CommentText, h.Response)
		}
	}

	prompt := fmt.Sprintf(`Você é o meu assistente e responde às mensagens que os inscritos do meu canal no Youtube me enviam. É um canal cristão protestante onde faço estudos bíblicos, tenho o devocional diário (AB7) e também um podcast de entrevistas.
	Você deve analisar o comentário abaixo, classificado como negativo e gerar uma resposta para ele que seja educada e não dê margem para o início de uma discussão.
	Não use adjetivos desnecessários e prefira respostas que não sejam muito longas, com até 20 palavras.
	Use sempre linguagem neutra e impessoal, sem tentar inferir se a pessoa é homem ou mulher. Nunca use termos com flexão de gênero como 'obrigado', 'obrigada', 'abençoado', 'abençoada', 'fico feliz que tenha gostado' (masc/fem). Prefira alternativas neutras como 'Agradeço pelo comentário', 'Que bom que gostou', 'Deus abençoe'.

	O comentário que você deve responder é este: "%s"
	O título do vídeo onde o comentário foi feito é: "%s"
	`, comment, videoTitle)

	if videoTranscript != "" {
		prompt += fmt.Sprintf("\nTRANSCRIÇÃO DO VÍDEO: Use esta transcrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoTranscript)
	}
	if videoDescription != "" {
		prompt += fmt.Sprintf("\nDESCRIÇÃO DO VÍDEO: Use esta descrição para entender o contexto do vídeo e dar uma resposta mais precisa:\n%s\n", videoDescription)
	}

	if historyContext != "" {
		prompt = fmt.Sprintf(`%s
		O histórico de interações anteriores com esta pessoa é o seguinte:
		%s
		Use esse histórico principalmente para evitar repetir respostas e para manter consistência no tom das respostas. Quando perceber que há vários comentários anteriores de tom positivo feitos pela mesma pessoa, você pode, mantendo sempre a linguagem neutra e impessoal e sem usar flexões de gênero, (1) reconhecer que ela já comenta com frequência, (2) agradecer de forma um pouco mais explícita e (3) adotar um tom ligeiramente mais acolhedor, sempre com poucas frases e sem exageros.
		`, prompt, historyContext)
	}

	var consistencyContext string
	if len(ragContext) > 0 {
		consistencyContext = "\nINSTRUÇÃO DE CONSISTÊNCIA: No passado, respondi a comentários similares (mesmo tema e sentimento) da seguinte forma:\n"
		for _, c := range ragContext {
			consistencyContext += c + "\n"
		}
		consistencyContext += "\nUse essas respostas como base de tom e doutrina para gerar a nova resposta para o comentário atual.\n"

		prompt += consistencyContext
	}

	if isMember {
		prompt += "\nNote que este usuário é membro do canal, considere isso ao dar a resposta, agradecendo o apoio.\n"
	}

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
		"sentimento": "positivo|neutro|negativo", // sentimento geral do comentário
		"tema": "Tema_do_Comentário" // tema principal do comentário
	}

	Identifique também o 'tema' principal do comentário. Use um dos seguintes temas pré-definidos para garantir consistência: [Teologia (Dízimo), Teologia (Graça), Teologia (Sábado), Escatologia, Estudo Bíblico (Geral), Devocional (AB7), Podcast (Entrevista), Saudação/Agradecimento, Crítica (Tom), Dúvida (Geral), Pedido de Oração, Outro].

	Exemplo de comentário e resposta esperada:

	Comentário: "Amém""
	Resposta: 
	{
		"nota": 5,
		"sentimento": "positivo",
		"tema": "Saudação/Agradecimento"
	}

	Comentário: "Isso é verdade, Deus é maravilhoso!"
	Resposta: 
	{
		"nota": 5,
		"sentimento": "positivo",
		"tema": "Saudação/Agradecimento"
	}

	Comentário: "Por que Deus permite tanto sofrimento no mundo?"
	Resposta: 
	{
		"nota": 1,
		"sentimento": "neutro"
		"tema": "Dúvida (Geral)"
	}

	Comentário: "Você não entende nada sobre a Bíblia."
	Resposta: 
	{
		"nota": 3,
		"sentimento": "negativo"
		"tema": "Crítica (Tom)"
	}

	Comentário: "Falou besteira."
	Resposta:
	{
		"nota": 4,
		"sentimento": "negativo"
		"tema": "Crítica (Tom)"
	}

	Comentário que deve ser analisado: "%s"
	`, comment)
	return prompt
}
