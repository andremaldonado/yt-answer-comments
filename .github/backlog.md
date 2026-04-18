# Backlog — answer-comments

## Em Andamento
<!-- O que está sendo trabalhado agora; deixe vazio se nada estiver em progresso -->

## Próximos Passos

1. **Pausa nos comentários sem resposta automática (modo auto)** — *Alta*
   - **Contexto:** No modo `-a`, comentários que não atingem o threshold de auto-publish (positivo + nota >= 4) não têm pausa — o fluxo vai direto pro menu de edição/ações sem dar tempo de ler o comentário. O usuário perde comentários sem perceber.
   - **Subtarefas:**
     - [ ] Mapear todos os paths em `handleUnansweredComment` que não resultam em auto-publish quando `opts.AutoAnswerMode == true`
     - [ ] Adicionar `ui.Countdown` (~30s) antes do prompt de ação nesses paths — Enter pula a espera e vai pro menu
     - [ ] Exibir no countdown o que faltou pro threshold (ex: "nota 3 — mínimo 4")

2. **Painel lateral no terminal** — *Alta*
   - **Contexto:** A tela atual mistura informações primárias e secundárias. A ideia é separar: tela principal com o que importa para a decisão (autor, comentário, sugestão, ações) e painel lateral com o contexto de suporte (título do vídeo, sentimento/nota/tema, RAG).
   - **Subtarefas:**
     - [ ] Detectar largura do terminal e dividir em duas colunas
     - [ ] Coluna principal (esquerda, ~60%): autor, comentário, sugestão de resposta, menu de ações
     - [ ] Coluna lateral (direita, ~40%): título do vídeo, badges de sentimento/nota/tema, barra de contexto RAG
     - [ ] Garantir fallback gracioso para terminais estreitos (< 100 cols): layout de coluna única

3. **Atualizar README** — *Baixa*
   - **Contexto:** A seção de estrutura do projeto está desatualizada — não reflete os pacotes `internal/ui`, `internal/service` e `internal/app` adicionados nas últimas refatorações.
   - **Subtarefas:**
     - [ ] Atualizar árvore de diretórios
     - [ ] Atualizar descrição de cada pacote

## Feito

- [2026-04-18] **[BUG] Goroutine vazada do Countdown consome stdin no comentário seguinte** — leitura de stdin movida para dentro de `ui.Countdown` com `done` channel; goroutine interna descarta o resultado via `select` quando o timer expira, eliminando o vazamento.
- [2026-04-18] **Countdown antes de publicar no auto-answer mode** — `ui.Countdown` com ticker + goroutine lendo stdin; Enter cai no fluxo de edição (`input = "E"`); últimos 60s piscam fundo vermelho (bloco `BLINK_ALERT` isolado para fácil remoção).
- [2026-04-06] **Refatoração da UI do terminal** — criação do pacote `internal/ui` com sistema completo de display ANSI (badges, headers, barras compactas de metadados e contexto, full-width dinâmico via `term.GetSize`); `comment_service.go` e `main.go` migrados dos `fmt.Printf` soltos para o novo pacote.

## Bloqueado
