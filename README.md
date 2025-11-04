# answer-comments

Ferramenta CLI para gerar e (opcionalmente) publicar respostas a comentários do seu canal YouTube usando o modelo Gemini (via `google.golang.org/genai`) e a YouTube Data API v3.

O programa busca comentários não respondidos do canal autenticado, gera uma sugestão de resposta com o LLM (Gemini) e pergunta interativamente se você deseja publicar a resposta.

## Requisitos

- Go 1.25.1 (conforme `go.mod`)
- Conta Google com um canal YouTube
- Credenciais OAuth 2.0 (arquivo `client_secret.json`) configuradas na Google Cloud Console para o uso da DataAPI V3 do YouTube
- Variável de ambiente `GEMINI_API_KEY` com a chave da API usada pelo pacote `google.golang.org/genai`

## Estrutura do projeto

```
cmd/
  answer-comments/
    main.go         # ponto de entrada da aplicação
internal/
  database/
    db.go          # gerenciamento de banco de dados SQLite
  llm/
    llm.go         # interação com a API do Gemini
  models/
    models.go      # estruturas de dados compartilhadas
  youtube/
    youtube.go     # interação com a API do YouTube
```

- `go.mod` / `go.sum` - dependências do projeto
- `members.csv` - caso queira identificar membros do canal (necessário exportar CSV diretamente do Youtube Studio pois a API de membros necessita de aprovação de um Youtube Partner Manager). Este arquivo é opcional.
- `comments.db` - banco de dados SQLite para armazenamento de histórico de comentários

## Configuração do Google API

1. Acesse a Google Cloud Console.
2. Crie um projeto (ou use um existente) e ative a API "YouTube Data API v3".
3. Em "APIs e serviços > Credenciais", crie credenciais do tipo "ID do cliente OAuth". Para este tipo de aplicação CLI/desktop, escolha "Aplicativo para desktop" ou "Outro" conforme necessário.
4. Baixe o arquivo JSON de credenciais e renomeie/posicione como `client_secret.json` na raiz do repositório. Nunca faça commit desse arquivo.
5. Configure a tela de consentimento OAuth se solicitado (pelo menos para uso em modo teste).

Observação: o programa usa o escopo `youtube.YoutubeForceSslScope` (acesso para publicar comentários em nome do canal autenticado)

## Obtendo a GEMINI_API_KEY

Obtenha a chave da API compatível com o cliente `google.golang.org/genai` (Gemini). Configure-a no seu ambiente antes de executar o programa:

Para `zsh` (temporário na sessão atual):

```bash
export GEMINI_API_KEY="sua_chave_aqui"
```

Para persistir, adicione a mesma linha ao seu `~/.zshrc`.

## Build e execução

Na raiz do projeto:

```bash
# Instalar dependências e executar diretamente
go run ./cmd/answer-comments

# Ou compilar e executar o binário
go build -o answer-comments ./cmd/answer-comments
./answer-comments
```

Na primeira execução, o aplicativo solicitará que você acesse uma URL para autorizar o acesso ao seu canal. Cole o código de autorização fornecido pelo navegador na linha de comando. O `token.json` será salvo automaticamente.

Fluxo de uso:
- O programa busca comentários não respondidos do canal autenticado.
- Para cada comentário não respondido, ele gera uma sugestão de resposta via Gemini.
- O programa exibe a sugestão (e uma nota de entendimento) e pergunta se você deseja publicar. Responda `S` para publicar, `N` para pular ou `Q` para sair.

## Observações de segurança

- Não compartilhe `client_secret.json` nem `token.json` publicamente.
- Guarde `GEMINI_API_KEY` em local seguro (variáveis de ambiente, cofre de segredos, etc.).

## Banco de dados e histórico

O programa agora utiliza um banco de dados SQLite (`comments.db`) para armazenar o histórico de comentários e respostas. Isso permite:

- Rastreamento completo de todas as interações
- Análise do histórico de interações com cada usuário
- Respostas mais contextualizadas baseadas em interações anteriores
- Melhoria na qualidade das respostas para membros do canal
- Construção de dataset para sistema RAG com categorização por temas

O banco armazena:
1. ID do comentário do YouTube
2. Autor do comentário
3. Texto original
4. Análise de sentimento
5. Nota de entendimento (1-5)
6. Tema do comentário
7. Resposta gerada
8. Se a resposta foi editada pelo usuário
9. Data/hora do comentário
10. Data/hora da resposta
11. ID do vídeo

### Temas de Categorização

Os comentários são automaticamente categorizados em temas. Os temas são configurados diretamente no prompt para a LLM.

Esta categorização é usada para:
- Construir um dataset estruturado para um sistema RAG
- Garantir consistência doutrinária nas respostas
- Analisar padrões de engajamento por tema
- Identificar tópicos que precisam de mais conteúdo ou esclarecimento

O histórico de interações é usado pelo LLM para:
- Evitar repetir respostas para o mesmo usuário
- Identificar padrões de comportamento
- Ajustar o tom das respostas baseado em interações anteriores
- Dar tratamento diferenciado para membros do canal

## Troubleshooting

- Erro: `Não foi possível ler o arquivo client_secret.json` — coloque `client_secret.json` na raiz do projeto e verifique permissões.
- Erro: `A variável de ambiente GEMINI_API_KEY não está configurada.` — exporte a variável antes de executar.
- Erro ao criar o serviço do YouTube / permissões insuficientes — verifique se a API YouTube Data v3 está habilitada e se as credenciais têm o escopo correto.
- Se o token não for salvo devido a permissão, verifique as permissões do diretório e execute com um usuário que possa criar arquivos.
- Erro ao criar/acessar o banco de dados — verifique se o diretório tem permissões de escrita e se o SQLite está instalado no sistema.
- Se o banco de dados ficar corrompido, exclua o arquivo `comments.db` e execute o programa novamente (um novo banco será criado).

## Contribuição

Pequenas melhorias e correções de bugs são bem-vindas. Abra uma issue ou pull request com descrição clara do problema/feature.

## Licença

Este projeto está licenciado sob a licença MIT — veja o arquivo `LICENSE` para detalhes.
