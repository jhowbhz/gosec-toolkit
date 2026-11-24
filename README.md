<div align="center">

# gosec-toolkit

**Kit de pentest em Go — 12 ferramentas de linha de comando que se encaixam, mais um painel web para rodar tudo pelo navegador.**

Recon → coleta → auditoria → dossiê. Cada peça é autossuficiente, passiva por padrão e some os segredos que encontra. Feito para o teste de segurança do **seu** serviço.

![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)
![Licença](https://img.shields.io/badge/licença-MIT-black)
![Dependências](https://img.shields.io/badge/deps-quase%20zero%20(stdlib)-2ea44f)
![Uso](https://img.shields.io/badge/uso-autorizado-9a6700)

</div>

---

Doze binários pequenos que não dependem um do outro em tempo de compilação — eles
conversam pelos **artefatos** que geram (`.jsonl`, `.db`, `.txt`, `.md`). Rode um,
jogue a saída no próximo, e no fim junte tudo num relatório. Ou abra o **painel**
e faça isso clicando.

```
 RECON            COLETA              AUDITORIA                      DOSSIÊ
 ─────            ──────              ─────────                      ──────
 certspy ─┐       download-dump ─┐    jsleak · spyder-credentials ─┐
 spyder  ─┼ URLs ─┤ proxyreverse ─┼── headeraudit · tlsscan      ──┼ apidiff ─┐
          │       │               │   jwtinspect · ratemeter      ─┘ (drift) ─┼─► evidence ─► relatório.md
          └───────┴───────────────┴────────────────────────────────────────── ┘
                              └──────────────── painel (orquestra tudo pela web) ───────────────┘
```

## As ferramentas

| Ferramenta | Fase | O que faz |
|---|---|---|
| **certspy** | recon | Acha domínios/subdomínios por Certificate Transparency (crt.sh) + SANs ao vivo; resolve DNS. |
| **spyder** | recon | Mapeia rotas e **endpoints de API reais** com Chrome headless (vê SPAs, captura XHR/fetch); dashboard ao vivo. |
| **download-dump** | coleta | Espelha o site para navegação **offline**; respeita `robots.txt`; **sinaliza** arquivos sensíveis sem baixá-los. |
| **proxyreverse** | coleta | Proxy reverso de interceptação: fica no meio e grava cada req/resp num log. |
| **jsleak** | auditoria | Varre arquivos e URLs atrás de segredos, chaves, tokens e endpoints vazados; mostra **redigido**. |
| **spyder-credentials** | auditoria | Scanner de segredos que **classifica**: `secreto` vs `publicável` (ex.: chave anon) vs `suspeito`. |
| **headeraudit** | auditoria | Audita postura HTTP: headers de segurança, cookies e reflexão de CORS; **nota A–F**. |
| **tlsscan** | auditoria | Inspeciona TLS/cert: versões, ciphers, cadeia, validade, HSTS; **nota A–F**. |
| **jwtinspect** | auditoria | Decodifica e audita JWTs (alg, claims, validade); testa **offline** se o HMAC é fraco. |
| **ratemeter** | auditoria | Mede rate-limit/quota com rampa **conservadora**; para no primeiro 429. Não é DoS. |
| **apidiff** | dossiê | Compara dois scans e mostra o **drift** da superfície (added/removed/changed). |
| **evidence** | dossiê | Lê os artefatos de todas e monta **um relatório Markdown**, com passe de redação. |
| **🖥️ painel** | — | App web local que roda qualquer ferramenta por formulário, com **saída ao vivo** e download de artefatos. |

## Começo rápido

Precisa de **Go 1.27+** (e Chrome, só para o `spyder`; e Python 3.11+, só para o `evidence`).

```bash
git clone https://github.com/jhowbhz/gosec-toolkit
cd gosec-toolkit

# rode uma ferramenta direto:
cd tlsscan && go run . --host=meusite.com.br && cd ..

# ou suba o painel e faça tudo pelo navegador:
cd painel && go build -o painel . && ./painel
# abra a URL que ele imprime (já vem com o token)
```

Um fluxo típico, encadeando pelos artefatos:

```bash
# 1. espelha o site                    2. varre o espelho atrás de segredos
cd download-dump && go run . --url=https://meusite.com.br/ --out=dump && cd ..
cd spyder-credentials && go run . ../download-dump/dump && cd ..
```

## O painel

Um binário Go que sobe um servidor **só em `127.0.0.1`**, protegido por **token**,
e executa as ferramentas por `argv` (nunca por shell). Formulário gerado do schema
de cada ferramenta, **streaming** da saída via SSE, histórico de execuções e
download dos artefatos — tudo confinado à pasta de cada run.

## Filosofia

- **Componível.** Nenhuma ferramenta importa a outra; elas se encaixam pelos
  arquivos que produzem. Troque qualquer peça.
- **Quase sem dependências.** Biblioteca padrão do Go na maioria; nada de
  framework. Binário pequeno, fácil de auditar.
- **Discreta e educada.** Passiva por padrão, respeita `robots.txt` onde
  rastreia, redige segredos na saída, e **não exfiltra** conteúdo sensível — a
  postura é *auditar e evidenciar*.
- **Honesta.** Cada ferramenta tem README, testes e licença próprios, e diz suas
  limitações.

## ⚠️ Uso responsável

Este kit vai de leitura passiva a **requisições ativas** e consulta a serviços de
terceiros. Use **somente** em serviços que são seus ou que você tem **autorização
explícita** para testar. Respeite o `robots.txt`, os Termos de Uso e a lei
aplicável (inclusive LGPD/PII). Não é ferramenta para mirar sistemas alheios.

## Build e testes

Cada ferramenta se compila e se testa sozinha na própria pasta:

```bash
cd <ferramenta> && go build . && go test ./...
```

`apidiff` e `spyder` usam o driver SQLite puro-Go `modernc.org/sqlite` (um
`go mod tidy` resolve). `evidence` roda só com Python 3.11+ (stdlib).

## Contribuindo

Issues e PRs são bem-vindos. Mantenha o padrão da casa: biblioteca padrão quando
der, testes antes do código, README por ferramenta e a postura de uso responsável.

## Licença

[MIT](proxyreverse/LICENSE) — cada ferramenta traz a sua.
