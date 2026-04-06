---
name: backlog
description: |
  Maintain the project backlog at `.github/backlog.md`. Activates EVERY session.
  Read backlog silently on start. Refine new items interactively before adding.
  Move completed items to Done with date. Keep the backlog as the single source
  of truth for what to build — the napkin covers how to build it.
---

# Backlog

You manage the project backlog at `.github/backlog.md`. This is about **what to
build**, not how — the napkin handles technical know-how.

**This skill is always active. Every session. No trigger required.**

## Session Start: Read Silently

First thing, every session — read `.github/backlog.md`. Internalize state.
Do not announce the read. Just apply what you know:

- What is currently in progress?
- What are the top priorities?
- Is there anything blocked?

If no backlog exists yet, create one using the structure below.

## When a New Item Appears

Trigger: user mentions a new feature, idea, improvement, or task to be done.

Before writing anything to the backlog, conduct a short refinement conversation.
Ask these questions **one at a time**, in sequence — do not dump them all at once:

1. **Contexto/motivação** — "Por que você quer isso? Que problema resolve ou que
   melhoria traz?"

2. **Subtarefas** — "Como você imagina os passos para implementar?" Suggest a
   breakdown if the user is unsure. Keep subtasks concrete and implementable.

3. **Prioridade** — Show the current "Próximos Passos" list and ask: "Onde você
   colocaria esse item em relação aos que já estão aqui?"

Only after all three questions are answered, write the refined item to the
backlog with full structure (see format below).

## During Work

- When starting on a backlog item, move it to "Em Andamento".
- If work is interrupted, note the current state briefly in the item.
- When an item is fully done, move it to "Feito" with today's date and a brief
  note on what was done.

## Backlog Format

```markdown
# Backlog — AB7 Publisher

## Em Andamento
<!-- O que está sendo trabalhado agora; deixe vazio se nada estiver em progresso -->

## Próximos Passos

1. **Título do item** — *Alta/Média/Baixa*
   - **Contexto:** Por que existe, que problema resolve
   - **Subtarefas:**
     - [ ] subtarefa concreta
     - [ ] subtarefa concreta
   - **Feito quando:** critério objetivo de conclusão (opcional)

## Feito

- [YYYY-MM-DD] **Título** — o que foi feito

## Bloqueado

- **Título** — o que está impedindo progresso
```

## Rules

- Never add an item without going through the refinement conversation first.
- Items in "Próximos Passos" must always have Contexto and at least one Subtarefa.
- Items migrated from elsewhere that lack refinement are marked *(a refinar)* —
  refine them when they become the focus.
- Keep "Próximos Passos" sorted by priority (highest first).
- "Feito" is append-only — never remove or edit past entries.
