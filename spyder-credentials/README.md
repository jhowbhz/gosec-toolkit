# spyder-credentials

Scanner de segredos de linha de comando, em Go. Você aponta pra um conteúdo
(uma pasta — ex.: o espelho do [`download-dump`](../download-dump) — arquivos,
URLs avulsas ou `stdin`) e ele **varre tudo e diz quais credenciais estão
expostas**: JWT, chaves de API, chaves privadas, tokens, strings de conexão.
Não executa JavaScript nem rastreia o site — é o motor de detecção; quem
descobre as páginas é o `download-dump`.

```
download-dump --url=https://meusite.com/ --out=dump   # espelha o site
spyder-credentials dump/                               # varre e reporta
```

Classifica cada achado pelo que importa num relatório de segredo:

- **secreto** — nunca deveria aparecer (service_role JWT, chave privada, `sk_live_`, string de conexão com senha…).
- **publicavel** — público por desenho (chave `sb_publishable_` do Supabase, `pk_live_` do Stripe, JWT `anon`). Informativo, não alarme.
- **suspeito** — parece segredo, precisa de conferência humana.

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/spyder-credentials@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/spyder-credentials
cd spyder-credentials
go build -o spyder-credentials .
```

## Uso

```bash
# varrer uma pasta (o espelho do download-dump, ou qualquer código)
spyder-credentials dump/

# varrer URLs avulsas (com auth, se precisar) — NÃO rastreia o site
spyder-credentials --url=https://meusite.com/app.js --header="Cookie: sid=..."

# via pipe
curl -s https://meusite.com/config.js | spyder-credentials --stdin

# só o que é realmente segredo (bom pra uma leitura rápida / CI)
spyder-credentials dump/ --only-secrets
```

Flags e caminhos podem vir em qualquer ordem (`spyder-credentials dump/ --only-secrets` funciona).

## Flags

| Flag | O que faz |
|---|---|
| `<caminho...>` | arquivos/pastas a varrer (recursivo; ignora binários) |
| `--url=URL` | URL avulsa a buscar e varrer (repetível; **não** rastreia o site) |
| `--stdin` | lê o conteúdo da entrada padrão |
| `--header="N: V"` | header de request para as URLs (`Authorization`, `Cookie`…) (repetível) |
| `--user-agent=UA` | User-Agent das buscas |
| `--insecure` | não valida o cert TLS ao buscar URLs |
| `--entropy` | liga o detector genérico de alta entropia (ruidoso) |
| `--show-full` | mostra o valor inteiro dos segredos (sem mascarar) |
| `--only-secrets` | reporta só a classe `secreto` |
| `--out=PATH` | arquivo-resumo (padrão: `_CREDENCIAIS-ENCONTRADAS.txt`) |
| `--log-file=PATH` | manifesto JSONL (padrão: `spyder-credentials-<ts>.jsonl`) |
| `--quiet` | não imprime o relatório no console |

Ao menos uma entrada (`<caminho>`, `--url` ou `--stdin`) é obrigatória.

## O que ele detecta

Detectores curados de alto sinal: **JWT** (classificado pela `role` do payload —
`service_role` é crítico, `anon`/`authenticated` é publicável; marca se expirou),
**chave privada** (`-----BEGIN … PRIVATE KEY-----`), **Supabase** (`sb_secret_`
crítico / `sb_publishable_` publicável), **AWS** (`AKIA…`), **Google** (`AIza…`),
**GitHub** (`ghp_…`), **Slack** (`xox…`), **Stripe** (`sk_live_` / `pk_live_`),
**SendGrid** (`SG.…`), **strings de conexão** com `usuário:senha@host`
(`postgres://`, `mysql://`, `mongodb://`, `redis://`) e **atribuições genéricas**
(`api_key=…`, `secret:…`, `password=…`) com filtro de ruído para placeholders e
código minificado. `--entropy` adiciona um detector genérico de cadeias de alta
entropia (mais achados, mais ruído).

## Redação

Por padrão os valores são **mascarados** — só as pontas e o tamanho
(`eyJhbGci…xcFAw (len 842)`) —, para o relatório não virar um depósito de
segredos em claro. `--show-full` revela o valor inteiro.

## Saída

- **Console:** achados agrupados do mais grave ao mais leve, com tipo, preview, local e nota.
- **Manifesto JSONL:** uma linha por achado (`detector`, `classe`, `severidade`, `preview`, `local`, `linha`, `ocorrencias`, `locais`, `extra`).
- **`_CREDENCIAIS-ENCONTRADAS.txt`:** resumo legível junto da varredura.
- **Exit code `2`** se houver qualquer achado `secreto` (gate de CI); senão `0`.

## Uso responsável

O `spyder-credentials` existe para você encontrar — e corrigir — segredos que o
**seu próprio** serviço expõe. Use **só** em conteúdo/alvo que é seu ou que você
tem **autorização explícita** para testar. Não é ferramenta para colher
credenciais de terceiros.

## Limitações conhecidas

- **Varredura estática por regex.** Pega segredos em texto (JS/HTML/JSON/.env/
  YAML/source maps); não executa JS nem avalia código. Conteúdo que só existe em
  tempo de execução não é visto.
- **Não rastreia o site.** Varre o que você der (pasta/URLs/stdin). Para cobrir
  um site inteiro, gere o espelho com o `download-dump` antes.
- **O detector genérico tem ruído.** Em bundles minificados ele pode marcar
  trechos de código — por isso esses achados ficam na classe `suspeito`
  (médio), nunca como `secreto`. Use `--only-secrets` para a leitura limpa.
- **Não valida assinatura de JWT** — classifica pelo payload, que é legível sem a chave.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o spyder-credentials .
```

## Licença

[MIT](LICENSE).
