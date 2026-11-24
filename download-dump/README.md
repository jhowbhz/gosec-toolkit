# download-dump

Raspador de linha de comando, em Go. Você passa um endereço e ele **baixa
tudo que encontrar** — páginas, JS, CSS, imagens, PDFs — montando um
**espelho navegável offline**: os links dentro do HTML/CSS são reescritos
para os arquivos locais, então você abre o `index.html` e navega sem
internet. Respeita o `robots.txt`.

```
VOCÊ PASSA:   --url=https://site.com/
                     │  rastreia dentro do escopo, respeitando robots.txt
                     ▼
ESPELHO:      dump-site.com-<ts>/site.com/index.html   ← abre no navegador, offline
MANIFESTO:    download-dump-<ts>.jsonl                 ← o que passou (status, hash, origem)
SENSÍVEIS:    dump-.../_ACHADOS-SENSIVEIS.txt           ← o que existe mas NÃO foi baixado
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/download-dump@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/download-dump
cd download-dump
go build -o download-dump .
```

## Uso rápido

```bash
download-dump --url=https://meusite.com.br/
```

Isso rastreia tudo dentro do host `meusite.com.br`, grava o espelho em
`dump-meusite.com.br-<timestamp>/` e o manifesto num `.jsonl`. Abra o
`index.html` dentro da pasta do host para navegar offline.

Área autenticada (seu próprio alvo), passando o cookie/sessão:

```bash
download-dump --url=https://meusite.com.br/painel \
  --header="Cookie: session=..." --scope=prefix
```

## Flags

| Flag | O que faz |
|---|---|
| `--url` | **(obrigatório)** URL semente, com scheme (`http://` ou `https://`) |
| `--out=DIR` | pasta de saída do espelho (padrão: `dump-<host>-<ts>`) |
| `--header="N: V"` | header de request, **repetível** (`Cookie`, `Authorization`, `User-Agent`...) |
| `--user-agent=UA` | User-Agent do crawler (padrão identificável) |
| `--scope=MODO` | até onde segue links: `host` (padrão), `domain` ou `prefix` |
| `--depth=N` | profundidade máxima de links; `-1` = ilimitado (padrão) |
| `--concurrency=N` | workers paralelos (padrão: 4) |
| `--delay=MS` | pausa mínima por host entre requests (o `Crawl-delay` do robots vence se maior) |
| `--max-body=KB` | pula arquivos maiores que isso; `0` = sem limite (padrão) |
| `--include=REGEX` | só segue/salva caminhos que casam |
| `--exclude=REGEX` | nunca segue/salva caminhos que casam |
| `--probe-sensitive` | sonda caminhos sensíveis comuns (`.env`, `.git/config`...) — **não baixa**, só avisa |
| `--ignore-robots` | ignora o `robots.txt` (use **só** em alvo autorizado) |
| `--insecure` | não valida o cert TLS do destino |
| `--log-file=PATH` | caminho do manifesto JSONL (padrão: `download-dump-<ts>.jsonl`) |
| `--quiet` | não imprime o resumo por recurso no console |

## Escopo (`--scope`)

Controla quais links o crawler **segue** (os de fora do escopo ficam como
links externos no espelho, apontando para a web):

- **`host`** (padrão) — só o mesmo host da semente (`meusite.com.br`).
- **`domain`** — o host e seus subdomínios (`*.meusite.com.br`).
- **`prefix`** — o mesmo host **e** só o que está sob o diretório da
  semente (ex.: semente `/painel/` → segue só `/painel/...`).

## robots.txt

Por padrão o `download-dump` **respeita** o `robots.txt` do host: busca
`/robots.txt`, aplica o grupo do seu User-Agent (ou `*`) e **não baixa**
caminhos em `Disallow`. O `Crawl-delay` é honrado como pausa mínima por host.
Falha ao buscar o robots = sem restrição (mesma postura de um navegador).

`--ignore-robots` desliga isso — **só** faça num alvo que é seu ou que você
tem autorização explícita para testar.

## Arquivos sensíveis — avisa, não baixa

A política é **conservadora**: arquivos que, num alvo real, não deveriam
estar expostos — `.env`, `.git/`, chaves (`.pem`, `.key`, `id_rsa`), dumps
(`.sql`), backups (`.bak`, `.zip`), logs, `wp-config.php` etc. — **não têm o
conteúdo baixado**. O crawler só confirma que existem (via `HEAD`) e os
registra no manifesto e no arquivo `_ACHADOS-SENSIVEIS.txt`, dentro da pasta
do espelho. Isso evita exfiltrar segredo durante um teste autorizado.

Com `--probe-sensitive`, ele ainda **sonda ativamente** uma lista embutida de
caminhos clássicos (`.env`, `.git/config`, `backup.zip`, `wp-config.php.bak`,
`phpinfo.php`...). A sonda também só confere existência — nunca baixa — e
respeita o `robots.txt` (caminho barrado não é sondado). Sondagem que dá
`404` é silenciosa.

## Espelho navegável offline

Depois de baixar, o `download-dump` faz um passe de reescrita sobre cada
HTML/CSS: referências a recursos que **foram baixados** passam a apontar para
o arquivo local, por caminho **relativo**. O que **não** foi baixado (link
externo, barrado por robots, sensível) fica com a URL/caminho original — a
página ainda o referencia, só não funciona offline. O conteúdo de `<script>`
e `<style>` é preservado como está (sem reescapar, para não quebrar JS/CSS).

## Manifesto (JSONL)

Um objeto JSON por linha, o registro do que passou:

```json
{"ts":"...","url":"https://site.com/app.js","status":200,
 "content_type":"application/javascript","size":1234,"sha256":"...",
 "saved_path":"site.com/app.js","from":"https://site.com/","depth":1,"duration_ms":8}
```

Pulados (`skipped`/`skip_reason`), erros (`error`) e sensíveis
(`sensitive`/`sensitive_note`, **sem** `saved_path`) também viram linha.

## Uso responsável

O `download-dump` baixa conteúdo de sites em escala. Use **só** em serviços
que são seus ou que você tem **autorização explícita** para raspar/testar.
Respeite o `robots.txt` (é o padrão), os Termos de Uso do site e a lei
aplicável (incluindo LGPD/PII). É uma ferramenta de estudo e de teste de
segurança autorizado — não a use para coletar dados de terceiros sem base.

## Limitações conhecidas

- **Não executa JavaScript.** É um crawler estático: em sites SPA (que
  montam a página no navegador), ele pega o HTML inicial e os assets
  referenciados, mas não o conteúdo que só aparece depois do JS rodar.
- **Links não-baixados ficam absolutos.** Externos, barrados por robots e
  sensíveis mantêm a referência original; offline eles não abrem (é o
  esperado).
- **`--scope=domain` aproxima o domínio** pelos dois últimos rótulos
  (`site.com`); não cobre sufixos de dois níveis como `co.uk`.
- **Bufferiza cada arquivo em memória** para gravar e hashear — não serve
  para baixar arquivos gigantes isolados; use `--max-body` como teto.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o download-dump .
```

## Licença

[MIT](LICENSE).
