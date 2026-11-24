# spyder

Mapeador de rotas de sites, em Go, com **área web ao vivo**. Renderiza as
páginas em Chrome headless (então enxerga SPAs), captura os links **e as
chamadas de rede** (os endpoints de API reais), e — no modo contínuo — faz
força bruta de caminhos com wordlists que ele **baixa sozinho**. Tudo aparece
em tempo real num dashboard e fica gravado em SQLite.

```
seed → Chrome headless → links do DOM + chamadas XHR/fetch ─┐
          │                                                  ├─► SQLite + dashboard ao vivo (SSE)
wordlists (baixadas) → prober HTTP → caminhos não-linkados ─┘
```

## Instalação

Com Go 1.27+ e **Chrome/Chromium instalado**:

```bash
go install github.com/jhowbhz/spyder@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/spyder
cd spyder
go build -o spyder .
```

## Uso

**Mapa rápido (segue links + captura APIs do load):**
```bash
spyder --url=https://site.com
# abra o dashboard: http://127.0.0.1:8750/
```

**Descoberta contínua (nunca para; usa wordlists):**
```bash
spyder --url=https://site.com --forever
# baixa o catálogo de wordlists sozinho (cacheado), varre em loop até Ctrl+C
```

Com suas próprias listas (ex.: SecLists) e mais polidez/velocidade:
```bash
spyder --url=https://site.com --forever \
  --wordlist=/caminho/seclists/common.txt \
  --delay=100ms --concurrency=5
```

## Flags

| Flag | O que faz |
|---|---|
| `--url` | **(obrigatório)** ponto de partida, com `http://` ou `https://` |
| `--out=PATH` | arquivo SQLite (padrão: `spyder-<ts>.db`) |
| `--dashboard=ADDR` | endereço da área web ao vivo (padrão: `127.0.0.1:8750`) |
| `--header "K: V"` | header extra em toda request (repetível) |
| `--max-pages=N` | teto de páginas renderizadas (padrão: 300) |
| `--max-depth=N` | profundidade a partir do seed (padrão: 4) |
| `--concurrency=N` | abas headless / probes em paralelo (padrão: 2) |
| `--delay=DUR` | espera entre requests por polidez (padrão: 300ms) |
| `--scope=MODE` | `same-host` (padrão) ou `subdomains` |
| `--ignore-robots` | ignora `robots.txt` (só no que é SEU/autorizado) |
| `--chrome=PATH` | caminho do Chrome (senão auto-detecta) |
| `--open` | abre o dashboard no navegador ao iniciar |
| `--api-only` | foco em API: grava só rotas de API (padrão: **on**; `--api-only=false` inclui o front) |
| **`--forever`** | descoberta contínua por wordlist, recursiva, até Ctrl+C |
| `--fetch` | baixa sozinho o catálogo de wordlists, cacheado (padrão: on) |
| `--wordlist=PATH` | wordlist local (repetível) |
| `--wordlist-url=URL` | wordlist remota pra baixar (repetível) |

## O que ele descobre

**O foco é a superfície de API, não o front** (`--api-only`, padrão on): o spyder
navega as páginas só pra capturar as chamadas de API que elas fazem, e grava só
as rotas de API. Chamada **XHR/fetch é API mesmo cross-origin** — o endpoint que
a SPA usa (ex.: `*.vercel.app`, Supabase) é exatamente o que interessa.

Tipos de rota:

- **xhr** — chamada de API (capturada da rede, ou caminho `/api`,`/rest`,`/graphql`,`.json`). É o alvo.
- **page** — página do mesmo host. Navegada pra achar APIs; só gravada com `--api-only=false`.
- **asset** — js/css/img. Só com `--api-only=false`.
- **external** — link pra outra origem (não-API). Só com `--api-only=false`.

As achadas por força bruta vêm com origem `wordlist`; no modo contínuo a varredura
inclui as bases comuns de API (`/api/`, `/api/v1/`, …).

## Wordlists (baixa sozinho)

No modo `--forever`, o spyder baixa um catálogo público (SecLists) na primeira
vez e **cacheia** (em `os.UserCacheDir()/spyder/wordlists`) — depois reusa sem
rede. Uma lista embutida de caminhos comuns funciona mesmo offline. Some tudo:
embutida + catálogo baixado + `--wordlist` (local) + `--wordlist-url` (remota),
deduplicado. Bônus: os `Disallow:` do `robots.txt` são bons candidatos a probar
com `--ignore-robots`.

## Dashboard ao vivo

Servidor embutido (uma página, sem dependências). Mostra contadores por tipo e
uma tabela que preenche em tempo real via SSE conforme as rotas são
descobertas, com filtro por texto e por tipo. Recarregar recupera o estado do
SQLite. Fica de pé durante e depois do crawl.

## Uso responsável

O `--forever` faz **requisições ativas** (força bruta) contra o alvo. Use **só**
em sites que são seus ou que você tem **autorização explícita** para testar.
Mantenha `--delay`/`--concurrency` civilizados para não derrubar o alvo. Respeita
`robots.txt` por padrão.

## Limitações conhecidas

- Captura o que carrega **ao navegar** em cada URL. Rotas que só disparam em
  **interação** (clicar, enviar form) não são acionadas no v1.
- Precisa do Chrome instalado (chromedp). Use `--chrome=PATH` se não for achado.
- Corpos não são guardados — o spyder mapeia **rotas**, não conteúdo.

## Desenvolvimento

```bash
go test ./...
go vet ./...
go build -o spyder .
```

## Licença

[MIT](LICENSE).
