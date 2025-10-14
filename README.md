# answer-comments

Ferramenta CLI para gerar e (opcionalmente) publicar respostas a comentários do seu canal YouTube usando o modelo Gemini (via `google.golang.org/genai`) e a YouTube Data API v3.

O programa busca comentários não respondidos do canal autenticado, gera uma sugestão de resposta com o LLM (Gemini) e pergunta interativamente se você deseja publicar a resposta.

## Requisitos

- Go 1.25.1 (conforme `go.mod`)
- Conta Google com um canal YouTube
- Credenciais OAuth 2.0 (arquivo `client_secret.json`) configuradas na Google Cloud Console
- Variável de ambiente `GEMINI_API_KEY` com a chave da API usada pelo pacote `google.golang.org/genai`

## Arquivos importantes

- `main.go` - código fonte principal
- `go.mod` / `go.sum` - dependências do projeto

## Configuração do Google API

1. Acesse a Google Cloud Console.
2. Crie um projeto (ou use um existente) e ative a API "YouTube Data API v3".
3. Em "APIs e serviços > Credenciais", crie credenciais do tipo "ID do cliente OAuth". Para este tipo de aplicação CLI/desktop, escolha "Aplicativo para desktop" ou "Outro" conforme necessário.
4. Baixe o arquivo JSON de credenciais e renomeie/posicione como `client_secret.json` na raiz do repositório. Nunca faça commit desse arquivo.
5. Configure a tela de consentimento OAuth se solicitado (pelo menos para uso em modo teste).

Observação: o programa usa o escopo `youtube.YoutubeForceSslScope` (acesso para publicar comentários em nome do canal autenticado).

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
go run main.go

# Ou compilar e executar o binário
go build -o answer-comments
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

## Logs e registro (arquivo de log)

O programa registra alguns eventos e, para cada sugestão de resposta com boa confiança, grava um registro em CSV:

- Arquivo: `comment_log.csv` (criado/append no diretório de execução)
- Formato: campos separados por ponto e vírgula (`;`) com as colunas na ordem:
	1. Autor (display name)
	2. Data/hora de publicação do comentário (formato: `02/01/2006 às 15:04`)
	3. Texto original do comentário
	4. Nota de entendimento atribuída pelo LLM (inteiro)
	5. Resposta sugerida pelo LLM

O código trata `;` (ponto e vírgula) dentro dos campos substituindo por vírgula para evitar quebra do separador. Cada linha é gravada com um timestamp (padrão `log` do Go) e os campos separados por `;`.

Observações sobre quando o arquivo é escrito:
- O registro só é escrito se a `nota` retornada pelo LLM for maior ou igual a 5. Comentários com `nota < 5` são automaticamente pulados (não são logados nem perguntados para publicação).
- Se houver falha ao abrir/escrever o arquivo `comment_log.csv`, o programa registra um aviso com `log.Printf` (não encerra a execução) e continua.

## Troubleshooting

- Erro: `Não foi possível ler o arquivo client_secret.json` — coloque `client_secret.json` na raiz do projeto e verifique permissões.
- Erro: `A variável de ambiente GEMINI_API_KEY não está configurada.` — exporte a variável antes de executar.
- Erro ao criar o serviço do YouTube / permissões insuficientes — verifique se a API YouTube Data v3 está habilitada e se as credenciais têm o escopo correto.
- Se o token não for salvo devido a permissão, verifique as permissões do diretório e execute com um usuário que possa criar arquivos.

## Contribuição

Pequenas melhorias e correções de bugs são bem-vindas. Abra uma issue ou pull request com descrição clara do problema/feature.

## Licença

Este projeto está licenciado sob a licença MIT — veja o arquivo `LICENSE` para detalhes.
