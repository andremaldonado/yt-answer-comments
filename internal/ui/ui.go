package ui

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/term"
)

// ── ANSI Codes ────────────────────────────────────────────────────────────────

const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Underline = "\033[4m"

	// Foreground colors
	FgBlack   = "\033[30m"
	FgRed     = "\033[31m"
	FgGreen   = "\033[32m"
	FgYellow  = "\033[33m"
	FgBlue    = "\033[34m"
	FgMagenta = "\033[35m"
	FgCyan    = "\033[36m"
	FgWhite   = "\033[37m"

	// Bright foreground
	FgBrightRed     = "\033[91m"
	FgBrightGreen   = "\033[92m"
	FgBrightYellow  = "\033[93m"
	FgBrightBlue    = "\033[94m"
	FgBrightMagenta = "\033[95m"
	FgBrightCyan    = "\033[96m"
	FgBrightWhite   = "\033[97m"

	// Background colors
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"

	// Bright background
	BgBrightBlue    = "\033[104m"
	BgBrightMagenta = "\033[105m"
	BgBrightCyan    = "\033[106m"
)

// ── Clear ─────────────────────────────────────────────────────────────────────

func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

// ── Width ─────────────────────────────────────────────────────────────────────

func termWidth() int {
	w, _, err := term.GetSize(1) // stdout fd
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// ── Box Drawing ───────────────────────────────────────────────────────────────

// PrintHeader renders a prominent full-width header box.
//
//	╔══════════════════════════════════════════════════════════════════════╗
//	║              Novo comentário não respondido encontrado              ║
//	╚══════════════════════════════════════════════════════════════════════╝
func PrintHeader(title string) {
	inner := termWidth() - 2 // subtract left+right border chars
	top := "╔" + strings.Repeat("═", inner) + "╗"
	bot := "╚" + strings.Repeat("═", inner) + "╝"

	// center the title
	padding := inner - len([]rune(title))
	left := padding / 2
	right := padding - left
	mid := "║" + strings.Repeat(" ", left) + title + strings.Repeat(" ", right) + "║"

	fmt.Println(Bold + FgBrightCyan + top + Reset)
	fmt.Println(Bold + FgBrightCyan + mid + Reset)
	fmt.Println(Bold + FgBrightCyan + bot + Reset)
}

// PrintSectionTitle renders a section divider with a label.
//
//	── Detalhes do comentário ──────────────────────────────────────────
func PrintSectionTitle(label string) {
	fmt.Println()
	line := "── " + label + " " + strings.Repeat("─", max(0, termWidth()-4-len([]rune(label))))
	fmt.Println(Bold + FgBrightBlue + line + Reset)
}

// PrintDivider prints a subtle horizontal rule.
func PrintDivider() {
	fmt.Println(Dim + FgWhite + strings.Repeat("─", termWidth()) + Reset)
}

// ── Badges ────────────────────────────────────────────────────────────────────

// SentimentBadge returns a colored inline badge for a sentiment value.
func SentimentBadge(sentimento string) string {
	switch strings.ToLower(sentimento) {
	case "positivo":
		return BgGreen + FgBlack + Bold + " ● POSITIVO " + Reset
	case "negativo":
		return BgRed + FgWhite + Bold + " ● NEGATIVO " + Reset
	case "neutro":
		return BgYellow + FgBlack + Bold + " ● NEUTRO   " + Reset
	default:
		return BgBlue + FgWhite + Bold + " ● " + strings.ToUpper(sentimento) + " " + Reset
	}
}

// NotaBadge returns a colored inline badge for the analysis score (1-5).
func NotaBadge(nota int) string {
	stars := strings.Repeat("★", nota) + strings.Repeat("☆", 5-nota)
	color := FgBrightYellow
	if nota <= 2 {
		color = FgBrightRed
	} else if nota >= 4 {
		color = FgBrightGreen
	}
	return color + Bold + fmt.Sprintf(" %s %d/5 ", stars, nota) + Reset
}

// ThemeBadge returns a styled inline theme tag.
func ThemeBadge(tema string) string {
	return FgBrightMagenta + "[🏷  " + tema + "]" + Reset
}

// MemberBadge returns a styled member badge string.
func MemberBadge() string {
	return BgBrightMagenta + FgWhite + Bold + " ⭐ MEMBRO " + Reset + " "
}

// ── Field Printing ────────────────────────────────────────────────────────────

// PrintField prints a labeled key-value line.
//   - Label  Value
func PrintField(label, value string) {
	fmt.Printf("  %s%-20s%s %s%s%s\n",
		Bold+FgBrightWhite, label+":", Reset,
		FgWhite, value, Reset,
	)
}

// PrintComment prints the comment text with a distinct style inside a subtle block.
func PrintComment(text string) {
	fmt.Println()
	fmt.Println("  " + FgBrightYellow + Bold + "💬 Comentário:" + Reset)
	lines := wrapText(text, termWidth()-6)
	for _, line := range lines {
		fmt.Println("  " + FgYellow + "> " + Reset + line)
	}
}

// PrintSuggestedAnswer prints the AI-generated suggested reply.
func PrintSuggestedAnswer(answer string) {
	lines := wrapText(answer, termWidth()-6)
	for _, line := range lines {
		fmt.Println("  " + FgCyan + Reset + line)
	}
}

// ── Status Messages ───────────────────────────────────────────────────────────

func Success(msg string) {
	fmt.Println("  " + FgBrightGreen + "✅ " + msg + Reset)
}

func Warning(msg string) {
	fmt.Println("  " + FgBrightYellow + "⚠️  " + msg + Reset)
}

func Error(msg string) {
	fmt.Println("  " + FgBrightRed + "❌ " + msg + Reset)
}

func Info(msg string) {
	fmt.Println("  " + FgBrightBlue + "ℹ️  " + msg + Reset)
}

func Muted(msg string) {
	fmt.Println("  " + Dim + FgWhite + msg + Reset)
}

// ── Action Prompt ─────────────────────────────────────────────────────────────

// PrintActionMenu prints a styled action menu and returns the prompt string.
func PrintActionMenu() {
	fmt.Println()
	PrintDivider()
	fmt.Printf("  %sAção:%s  ", Bold+FgBrightWhite, Reset)
	fmt.Printf("%s[S]%s Publicar   ", BgGreen+FgBlack+Bold, Reset)
	fmt.Printf("%s[E]%s Editar   ", BgBrightBlue+FgWhite+Bold, Reset)
	fmt.Printf("%s[N]%s Pular   ", BgYellow+FgBlack+Bold, Reset)
	fmt.Printf("%s[Q]%s Sair", BgRed+FgWhite+Bold, Reset)
	fmt.Printf(" %s→ %s", FgBrightCyan+Bold, Reset)
}

// PrintEditPrompt prints a styled prompt for manual answer input.
func PrintEditPrompt() {
	fmt.Println()
	fmt.Printf("  %s✏️  Digite sua resposta:%s\n", Bold+FgBrightBlue, Reset)
	fmt.Printf("  %s→ %s", FgBrightCyan+Bold, Reset)
}

// ── Searching Banner ──────────────────────────────────────────────────────────

func PrintSearchingBanner() {
	fmt.Println()
	fmt.Println(Dim + FgCyan + "  🔍 Buscando novos comentários não respondidos..." + Reset)
	fmt.Println()
}

// ── Mode Banners ──────────────────────────────────────────────────────────────

func PrintModeBanner(icon, title, description string, bgColor string) {
	inner := termWidth() - 2
	top := "╔" + strings.Repeat("═", inner) + "╗"
	bot := "╚" + strings.Repeat("═", inner) + "╝"

	titleStr := icon + " " + title
	padding := inner - len([]rune(titleStr))
	left := padding / 2
	right := padding - left
	midTitle := "║" + strings.Repeat(" ", left) + titleStr + strings.Repeat(" ", right) + "║"

	padding2 := inner - len([]rune(description))
	left2 := padding2 / 2
	right2 := padding2 - left2
	midDesc := "║" + strings.Repeat(" ", left2) + description + strings.Repeat(" ", right2) + "║"

	fmt.Println(bgColor + Bold + top + Reset)
	fmt.Println(bgColor + Bold + midTitle + Reset)
	fmt.Println(bgColor + Dim + midDesc + Reset)
	fmt.Println(bgColor + Bold + bot + Reset)
	fmt.Println()
}

// ── Comment Meta Bar ──────────────────────────────────────────────────────────

// PrintCommentMeta renders a compact single-line summary of comment metadata.
//
// Example:
//
//	📹 Título do vídeo  ·  👤 ⭐ MEMBRO  Nome  ·  📅 03/04/2026 às 14:30
func PrintCommentMeta(videoTitle, authorLine, date string) {
	sep := Dim + FgWhite + "  ·  " + Reset

	fmt.Println()
	fmt.Println("  " + FgBrightWhite + Bold + "📹" + Reset + " " + FgWhite + videoTitle + Reset)
	fmt.Println("  " + FgBrightWhite + Bold + "👤" + Reset + " " + FgWhite + authorLine + Reset + sep + Dim + FgWhite + "📅 " + date + Reset)
}

// ── Context Bar ───────────────────────────────────────────────────────────────

// PrintContextBar renders a compact single-line summary of available context.
//
//	transcriptLen: >0 = char count, -1 = fetch failed, 0 = not fetched
//	authorCount:   number of past interactions with this author
//	pastCount:     number of similar past answers (RAG)
//
// Example output:
//
//	📄 4.2k chars  ·  👥 3 interações  ·  🗂 5 similares
func PrintContextBar(transcriptLen, authorCount, pastCount int) {
	sep := Dim + FgWhite + "  ·  " + Reset

	// ── Transcript ────────────────────────────────────────────────────────────
	var transcriptPart string
	switch {
	case transcriptLen > 0:
		kb := float64(transcriptLen) / 1000
		transcriptPart = FgBrightCyan + Bold + "📄" + Reset + " " + FgBrightCyan + fmt.Sprintf("%.1fk chars", kb) + Reset
	case transcriptLen == -1:
		transcriptPart = Dim + FgWhite + "📄 indisponível" + Reset
	default:
		transcriptPart = Dim + FgWhite + "📄 —" + Reset
	}

	// ── Author history ────────────────────────────────────────────────────────
	var authorPart string
	if authorCount > 0 {
		authorPart = FgBrightBlue + Bold + "👥" + Reset + " " + FgBrightBlue + fmt.Sprintf("%d interaç", authorCount)
		if authorCount == 1 {
			authorPart += "ão" + Reset
		} else {
			authorPart += "ões" + Reset
		}
	} else {
		authorPart = Dim + FgWhite + "👥 —" + Reset
	}

	// ── Similar answers ───────────────────────────────────────────────────────
	var pastPart string
	if pastCount > 0 {
		pastPart = FgBrightMagenta + Bold + "🗂" + Reset + "  " + FgBrightMagenta + fmt.Sprintf("%d similar", pastCount)
		if pastCount != 1 {
			pastPart += "es" + Reset
		} else {
			pastPart += Reset
		}
	} else {
		pastPart = Dim + FgWhite + "🗂  —" + Reset
	}

	fmt.Println("  " + transcriptPart + sep + authorPart + sep + pastPart)
}

// ── Countdown ─────────────────────────────────────────────────────────────────

// Countdown exibe um contador regressivo inline, atualizando a cada segundo.
// Retorna true se o tempo esgotou normalmente, false se o usuário cancelou (Enter).
func Countdown(d time.Duration, cancelCh <-chan struct{}) bool {
	remaining := d
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		mins := int(remaining.Minutes())
		secs := int(remaining.Seconds()) % 60
		label := fmt.Sprintf(" ⏱  Publicando em %d:%02d... [Enter para editar] ", mins, secs)

		// === BLINK_ALERT: pisca fundo vermelho no último minuto; remova este bloco se não quiser ===
		if remaining <= time.Minute {
			blinkOn := (int(remaining.Seconds()) % 2) == 1
			if blinkOn {
				label = BgRed + Bold + label + Reset
			} else {
				label = Bold + label + Reset
			}
		}
		// === fim BLINK_ALERT ===

		fmt.Printf("\r  %s   ", label)

		select {
		case <-cancelCh:
			fmt.Println()
			return false
		case <-ticker.C:
			remaining -= time.Second
			if remaining <= 0 {
				fmt.Println()
				return true
			}
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// wrapText breaks a string into lines of at most width runes.
func wrapText(text string, width int) []string {
	words := strings.Fields(text)
	var lines []string
	current := ""

	for _, word := range words {
		if current == "" {
			current = word
		} else if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
