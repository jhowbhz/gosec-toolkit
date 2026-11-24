# headeraudit

Auditor de linha de comando, em Go, da **postura HTTP** de uma lista de URLs
que **você fornece** (ele não descobre nem rastreia nada). Para cada URL faz
um `GET` e um `OPTIONS` e confere headers de segurança, flags de cookie e
reflexão de CORS. Cada URL recebe uma **nota de A a F**. Só `GET`/`OPTIONS`
passivos.

```
URLs (--url / --urls-file / --jsonl)
        │
        ▼
  GET  ──► headers de segurança + Set-Cookie ─┐
  OPTIONS + Origin de teste ──► CORS          ├─► checks ─► NOTA (A..F)
                                              ┘               │
                                                              ▼
                              console (nota + achados por severidade)
                              headeraudit-<ts>.jsonl (um objeto por URL)
                              resumo final (distribuição de notas)
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/headeraudit@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/headeraudit
cd headeraudit
go build -o headeraudit .
```

## Uso rápido

```bash
headeraudit --url=https://meusite.com.br/ --url=https://meusite.com.br/login
```

Lista em arquivo (uma URL por linha, `#` comenta) e área logada:

```bash
headeraudit --urls-file=urls.txt --header="Cookie: session=..." --delay=200
```

Reaproveitando o que o `download-dump` já mapeou:

```bash
headeraudit --jsonl=download-dump-20260101-120000.jsonl --concurrency=2
```

## Flags

Ao menos uma fonte de URLs é obrigatória.

| Flag | O que faz |
|---|---|
| `--url=URL` | URL a auditar, com scheme (`http://` ou `https://`); **repetível** |
| `--urls-file=PATH` | arquivo com uma URL por linha |
| `--jsonl=PATH` | extrai URLs únicas (campo `url`) de um manifesto do `download-dump` |
| `--header="N: V"` | header extra em toda request, **repetível** (ex.: `Cookie` de área logada) |
| `--origin=ORIGIN` | Origin usado no teste de CORS (padrão: `https://attacker.example`) |
| `--concurrency=N` | requests em paralelo (padrão: 4, mínimo 1) |
| `--delay=MS` | pausa mínima entre requests ao mesmo host (padrão: 0) |
| `--timeout=S` | timeout por request, em segundos (padrão: 15) |
| `--insecure` | não valida o cert TLS do destino |
| `--log-file=PATH` | JSONL de saída (padrão: `headeraudit-<ts>.jsonl`) |
| `--quiet` | não imprime a linha por URL no console |

## O que é checado

Severidades: `critical`, `high`, `medium`, `low`, `info`.

**Headers de segurança** (resposta do `GET`)

| Check | Achado | Severidade |
|---|---|---|
| `Strict-Transport-Security` | ausente (só cobrado em https) | high |
| `Content-Security-Policy` | ausente | medium |
| `X-Frame-Options` | ausente **e** sem `frame-ancestors` no CSP | medium |
| `X-Content-Type-Options` | diferente de `nosniff` | low |
| `Referrer-Policy` | ausente | low |
| `Permissions-Policy` | ausente | info |

**Cookies** (cada `Set-Cookie` do `GET`)

| Achado | Severidade |
|---|---|
| sem `Secure` (só cobrado em https) | high |
| sem `HttpOnly` | medium |
| `SameSite` ausente, ou `SameSite=None` sem `Secure` | medium |

**CORS** (resposta ao `OPTIONS` com o `--origin` de teste)

| Achado | Severidade |
|---|---|
| `Access-Control-Allow-Origin` reflete a origem arbitrária | high |
| ...e `Access-Control-Allow-Credentials: true` junto | critical |

## Notas

A nota da URL é a **pior severidade** encontrada, não uma média: um achado
crítico não é compensado por cinco headers corretos. Achados `info` não
derrubam a nota.

| Nota | Pior severidade |
|---|---|
| A | nenhuma (ou só `info`) |
| B | low |
| C | medium |
| D | high |
| F | critical |

URL que deu erro de rede (timeout, DNS, TLS) **não recebe nota**: vai para o
JSONL com o campo `error` e conta à parte no resumo.

## Redirects

Segue até 5 redirects. Os headers avaliados são os da **última resposta**, e
o `final_url` no JSONL mostra onde a URL pousou (o `status` é o dessa
resposta). Passou do limite, avalia a última resposta recebida.

## Relatório (JSONL)

Um objeto por URL:

```json
{"ts":"2026-01-01T12:00:00Z","url":"https://site.com/","status":200,"grade":"D",
 "issues":[{"check":"Strict-Transport-Security","severity":"high","present":false,
 "detail":"HSTS ausente: ..."}],"duration_ms":120}
```

## Uso responsável

Audite **só** URLs que são suas ou que você tem **autorização explícita**
para testar. A ferramenta é passiva (só `GET`/`OPTIONS`, nada de payload),
mas ainda faz requests ao servidor: seja educado com `--delay` e
`--concurrency`, principalmente em ambiente de produção. É uma ferramenta de
estudo e de teste de segurança autorizado.

## Limitações conhecidas

- **Não renderiza JavaScript.** Avalia os headers que o servidor devolve na
  resposta HTTP; headers colocados por meta tags ou por JS não são vistos.
- **CSP só por presença.** Não interpreta o conteúdo da política: um CSP com
  `unsafe-inline` e `*` passa igual a um CSP restritivo.
- **CORS testado com um único Origin.** Servidores que refletem só origens
  que casam com um padrão (ex.: qualquer subdomínio, ou `null`) podem passar
  despercebidos; rode de novo com outro `--origin`.
- **Não descobre URLs.** Audita só o que você lista; páginas não listadas
  ficam fora da nota.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o headeraudit .
```

## Licença

[MIT](LICENSE).
