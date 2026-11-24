# jsleak

Auditor de linha de comando, em Go. Você aponta para **arquivos locais** (por
exemplo o espelho gerado pelo `download-dump`) e/ou **URLs ao vivo** e ele
procura **segredos, chaves, tokens, JWTs e strings de conexão vazados**, usando
um catálogo determinístico de regex. Reporta onde achou e o que achou **já
redigido** — só o prefixo do segredo aparece. Nada é enviado a lugar nenhum: o
achado fica no console e num JSONL local.

```
FONTES:     --dir=espelho/   --file=app.js   --url=https://site.com/app.js (GET passivo)
                  │  linha a linha
                  ▼
CATÁLOGO:   AWS · Google · GitHub · Slack · JWT · chave privada · Bearer ·
            connection string · basic-auth em URL · atribuições · Supabase
                  │  casou? redige o segredo (prefixo + …[N])
                  ▼
ACHADOS:    console:   CRITICAL aws-access-key  file:espelho/app.js:12  AKIA…[16]
                  │
                  ▼
MANIFESTO:  jsleak-<ts>.jsonl   ← local, um achado por linha (já redigido)
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/jsleak@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/jsleak
cd jsleak
go build -o jsleak .
```

## Uso rápido

Varrer o espelho baixado pelo `download-dump`:

```bash
jsleak --dir=dump-meusite.com.br-20260101-120000
```

Varrer um arquivo e uma URL do seu próprio serviço:

```bash
jsleak --file=build/app.js \
  --url=https://meusite.com.br/static/app.js \
  --header="Cookie: session=..."
```

## Flags

| Flag | O que faz |
|---|---|
| `--dir=PATH` | varre um diretório recursivamente, **repetível** |
| `--file=PATH` | varre um arquivo (qualquer extensão), **repetível** |
| `--url=URL` | baixa e varre uma URL via GET passivo; exige scheme `http://` ou `https://`, **repetível** |
| `--header="N: V"` | header das requests de `--url`, **repetível** |
| `--ext=LIST` | extensões varridas em `--dir`, csv (padrão: `js,html,json,txt,css,map,env,xml,yml,yaml,ts`) |
| `--max-body=KB` | ignora arquivo/resposta maior que isso; `0` = sem limite (padrão) |
| `--context=N` | chars de contexto em volta do match (padrão: 40) |
| `--redact` | redige o segredo na saída (**padrão: ligado**) |
| `--no-redact` | mostra o match inteiro — só no **seu** alvo |
| `--timeout=S` | timeout por request de `--url`, em segundos (padrão: 15) |
| `--insecure` | não valida o cert TLS do destino nas URLs |
| `--log-file=PATH` | manifesto JSONL (padrão: `jsleak-<ts>.jsonl`) |
| `--quiet` | não imprime cada achado no console (o resumo final continua) |

É preciso pelo menos uma fonte (`--dir`, `--file` ou `--url`). O filtro
`--ext` vale só para o que é encontrado dentro de `--dir`: um `--file` é pedido
explícito e é varrido qualquer que seja o nome. Arquivos binários (com byte
NUL no começo) são pulados.

## O catálogo

Cada regra tem nome, severidade (`critical`, `high`, `medium`, `low`), um
padrão e uma nota em pt-BR dizendo o que fazer:

| Regra | Severidade | Reconhece |
|---|---|---|
| `aws-access-key` | critical | `AKIA` + 16 caracteres |
| `github-token` | critical | `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` |
| `private-key` | critical | cabeçalho `-----BEGIN ... PRIVATE KEY-----` |
| `connection-string` | critical | `postgres://`, `mysql://`, `mongodb://`, `redis://`, `amqp://` com `usuário:senha@` |
| `supabase-service-role` | critical | JWT com `"role":"service_role"` |
| `google-api-key` | high | `AIza` + 35 caracteres |
| `slack-token` | high | `xoxb-`, `xoxp-`, `xoxa-`... |
| `bearer-token` | high | `Bearer <token longo>` |
| `url-basic-auth` | high | `https://usuário:senha@host` |
| `jwt` | medium | três segmentos `eyJ...` |
| `supabase-key` | medium | `sb_publishable_...`, `sb_secret_...` |
| `generic-secret` | low | `api_key`, `secret`, `token`, `password`, `authorization` atribuídos a um valor |

As regras ancoradas em prefixo fixo (AWS, GitHub, Slack) têm pouco falso
positivo; as genéricas (`generic-secret`, `bearer-token`) pegam mais coisa e
por isso ficam com severidade menor — confira o valor antes de agir.

## Redação — por que nunca mostra o segredo inteiro

Um relatório de auditoria que contém a credencial completa **é** um vazamento:
o console vai para o histórico do terminal, o JSONL vai para um repositório ou
um ticket. Por isso, com `--redact` (padrão), o `jsleak` mostra só os primeiros
4 caracteres e quantos foram escondidos:

```
AKIAIOSFODNN7EXAMPLE   →   AKIA…[16]
```

O prefixo basta para o dono reconhecer o tipo e achar a credencial (`grep` no
arquivo, na linha indicada); não basta para usá-la. Segredos curtos mostram
menos (um quarto) para o prefixo nunca ser metade do valor.

A redação vale também para o **contexto** em volta do match, inclusive contra
um segredo vizinho na mesma linha (comum em JS minificado): todos os
casamentos da linha são redigidos antes de recortar o contexto.

`--no-redact` existe para quando o alvo é seu e você quer ver o valor. O banner
avisa quando está ligado, porque o JSONL passa a conter segredos em texto puro.

## Manifesto (JSONL)

Um objeto JSON por linha:

```json
{"ts":"2026-01-01T12:00:00Z","source":"file:espelho/app.js","line":12,
 "rule":"aws-access-key","severity":"critical","match":"AKIA…[16]",
 "context":"var k=\"AKIA…[16]\",","note":"Chave de acesso AWS; revogue..."}
```

`source` é `file:<caminho>` para arquivos e a própria URL para `--url`. No fim,
o console traz o total por severidade.

## Uso responsável

O `jsleak` é uma ferramenta de **auditoria**, não de exfiltração. Use **só**
em serviços que são seus ou que você tem **autorização explícita** para testar.
As URLs são lidas com um GET passivo — sem seguir links, sem POST, sem
tentativa de usar a credencial encontrada. O que ele acha não é enviado a
lugar nenhum; fica no console e no arquivo local. Trate o manifesto como
material sensível mesmo redigido, e respeite a lei aplicável (incluindo LGPD).
Achou uma credencial válida? O próximo passo é **revogar e rotacionar**, não
testá-la.

## Limitações conhecidas

- **Regex tem falso positivo e falso negativo.** Um valor de exemplo na
  documentação casa; um segredo sem formato reconhecível (senha comum em
  variável com outro nome) passa batido. O resultado é um ponto de partida
  para revisão humana.
- **Não executa JavaScript.** Segredo montado em runtime (concatenado, vindo
  de `atob`, carregado depois por XHR) não aparece no texto estático.
- **Não decodifica nada.** Base64, strings escapadas e `.map` comprimidos não
  são abertos, exceto o caso embutido do `service_role` dentro de JWT.
- **Linha a linha.** O corpo de uma chave privada (as linhas depois do
  cabeçalho) não é inspecionado: o achado é o cabeçalho `BEGIN`.
- **Não valida o segredo.** Não testa se a chave ainda funciona — de
  propósito.
- **Bufferiza cada arquivo/resposta em memória**; use `--max-body` como teto.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o jsleak .
```

## Licença

[MIT](LICENSE).
