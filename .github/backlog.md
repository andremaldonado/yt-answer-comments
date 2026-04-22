# Backlog — answer-comments

## Em Andamento

## Próximos Passos

1. **Painel lateral no terminal** — *Alta*
   - **Contexto:** A tela atual mistura informações primárias e secundárias. A ideia é separar: tela principal com o que importa para a decisão (autor, comentário, sugestão, ações) e painel lateral com o contexto de suporte (título do vídeo, sentimento/nota/tema, RAG).
   - **Subtarefas:**
     - [ ] Detectar largura do terminal e dividir em duas colunas
     - [ ] Coluna principal (esquerda, ~60%): autor, comentário, sugestão de resposta, menu de ações
     - [ ] Coluna lateral (direita, ~40%): título do vídeo, badges de sentimento/nota/tema, barra de contexto RAG
     - [ ] Garantir fallback gracioso para terminais estreitos (< 100 cols): layout de coluna única

2. **Atualizar README** — *Baixa*
   - **Contexto:** A seção de estrutura do projeto está desatualizada — não reflete os pacotes `internal/ui`, `internal/service` e `internal/app` adicionados nas últimas refatorações.
   - **Subtarefas:**
     - [ ] Atualizar árvore de diretórios
     - [ ] Atualizar descrição de cada pacote

## Feito

- [2026-04-20] **[BUG] Comentários pulados silenciosamente em queda de rede** — `isNetworkError` + `os.Exit(-1)` no outer loop.
- [2026-04-20] **[BUG] Double Enter no prompt de próximo lote (AutoAnswerMode)** — race condition entre goroutine de stdin e `reader.ReadString`; prompt agora lê de `stdinCh` em AutoAnswerMode.
- [2026-04-18] **Pausa nos comentários sem auto-publish (modo `-a`)** — `ui.Countdown(30s)` adicionado antes do menu de ações quando threshold não é atingido (path `shouldSuggestAnswer` e path `suggestedAnswer == ""`); `Countdown` passou a aceitar `msg string` para exibir o motivo (ex: "Nota 3 (mínimo 4) —").
- [2026-04-18] **[BUG] Goroutine vazada do Countdown consome stdin no comentário seguinte** — leitura de stdin movida para dentro de `ui.Countdown` com `done` channel; goroutine interna descarta o resultado via `select` quando o timer expira, eliminando o vazamento.
- [2026-04-18] **Countdown antes de publicar no auto-answer mode** — `ui.Countdown` com ticker + goroutine lendo stdin; Enter cai no fluxo de edição (`input = "E"`); últimos 60s piscam fundo vermelho (bloco `BLINK_ALERT` isolado para fácil remoção).
- [2026-04-06] **Refatoração da UI do terminal** — criação do pacote `internal/ui` com sistema completo de display ANSI (badges, headers, barras compactas de metadados e contexto, full-width dinâmico via `term.GetSize`); `comment_service.go` e `main.go` migrados dos `fmt.Printf` soltos para o novo pacote.

## Bloqueado
