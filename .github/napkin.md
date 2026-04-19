# Napkin Runbook

## Curation Rules
- Re-prioritize on every read.
- Keep recurring, high-value notes only.
- Max 10 items per category.
- Each item includes date + "Do instead".

## Execution & Validation (Highest Priority)

- [2026-04-18] Atualizar backlog e napkin ao fim de cada tarefa — não só quando explicitamente pedido.
  Do instead: ao marcar item como feito no backlog, revisar o napkin na mesma resposta.

- [2026-04-18] Instrução em CLAUDE.md não é suficiente para garantir ações de fim de sessão — modelo esquece.
  Do instead: use Stop hook em settings.local.json para forçar lembrete via systemMessage no fim de cada resposta.

## Shell & Command Reliability

## Domain Behavior Guardrails

- [2026-04-18] Goroutines bloqueadas em `bufio.Reader.ReadString` não podem ser canceladas — `done` channel não adianta porque a goroutine só chega no `select` depois que o I/O retornar.
  Do instead: criar UMA goroutine permanente de leitura de stdin no início da sessão, alimentando um `chan string`. Countdown e qualquer outro leitor consomem o channel — nunca criam goroutines próprias de I/O.

- [2026-04-18] Modos de operação têm UX diferente — propor "sempre mostrar menu" quebra a lógica do AutoAnswerMode que exige countdown com auto-skip.
  Do instead: antes de propor mudança de fluxo, mapear todos os modos (Manual, AutoAnswer, padrão) e garantir que a mudança respeita o comportamento esperado de cada um.

## User Directives

- [2026-04-18] Ao adicionar flag/feature, cobrir TODOS os pontos afetados na mesma entrega — exemplos de uso, help text, referências no README.
  Do instead: antes de declarar pronto, varrer flag usage block, exemplos e help. Tratar como parte da mesma tarefa.

- [2026-04-18] Não adicionar UI/banners/output visual que não foi pedido explicitamente.
  Do instead: se um banner seria extensão natural, perguntar antes de implementar.
