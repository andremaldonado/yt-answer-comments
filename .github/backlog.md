# Backlog — answer-comments

## Em Andamento
<!-- O que está sendo trabalhado agora; deixe vazio se nada estiver em progresso -->

## Próximos Passos

1. **Countdown antes de publicar no auto-answer mode** — *Alta*
   - **Contexto:** No modo auto-answer, o comentário é publicado automaticamente após um delay (atualmente 3 min). O usuário precisa ver um contador regressivo para saber quando vai publicar e ter chance de cancelar — também reduz risco de rate-limit/bloqueio do YT por ações rápidas demais.
   - **Subtarefas:**
     - [ ] Criar `ui.Timer` (ou similar) que exibe countdown atualizado a cada segundo (`Publicando em 2:47...`)
     - [ ] Integrar no fluxo de auto-answer antes do publish
     - [ ] Permitir cancelamento durante o countdown (ex: `Ctrl+C`)

2. **Painel lateral no terminal** — *Alta*
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

- [2026-04-06] **Refatoração da UI do terminal** — criação do pacote `internal/ui` com sistema completo de display ANSI (badges, headers, barras compactas de metadados e contexto, full-width dinâmico via `term.GetSize`); `comment_service.go` e `main.go` migrados dos `fmt.Printf` soltos para o novo pacote.

## Bloqueado
