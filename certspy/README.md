# certspy

Ferramenta de linha de comando, em Go, que **descobre domínios e subdomínios
a partir de certificados SSL públicos**. Combina duas fontes — o Certificate
Transparency (via crt.sh) e os SANs do certificado que o host apresenta ao
vivo —, deduplica, marca quais nomes estão no ar e grava `.txt`, `.jsonl` e
um relatório `.md`.

```
VOCÊ PASSA:   --domain=meusite.com.br  [--host=www.meusite.com.br]
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
   crt.sh (CT público)     handshake TLS ao vivo
   name_value + not_before  DNSNames + CN do cert
         └───────────┬───────────┘
                     ▼
        mescla + dedup (guarda as fontes e o not_before mais antigo)
                     ▼
        marca o que está sob o apex do alvo
                     ▼
        resolve DNS  → vivo = tem A/AAAA
                     ▼
   certspy-<ts>.txt   certspy-<ts>.jsonl   certspy-<ts>.md
                     ▼
        [opcional, OFF por padrão] POST --publish-url
```

## Instalação

Com Go instalado (1.27+), só biblioteca padrão:

```bash
go install github.com/jhowbhz/certspy@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/certspy
cd certspy
go build -o certspy .
```

## Uso rápido

```bash
certspy --domain=meusite.com.br
```

Consulta o CT por `%.meusite.com.br` e por `meusite.com.br` (o apex),
resolve o DNS de cada nome e grava as três saídas na pasta atual.

Combinando as duas fontes:

```bash
certspy --domain=meusite.com.br --host=www.meusite.com.br --host=api.meusite.com.br:8443
```

Só o certificado ao vivo, sem tocar o crt.sh:

```bash
certspy --host=www.meusite.com.br --no-ct
```

## Flags

| Flag | O que faz |
|---|---|
| `--domain=DOM` | domínio-alvo para consultar no CT, **repetível** |
| `--host=HOST[:PORT]` | host para extrair SANs do cert ao vivo, **repetível** |
| `--no-ct` | não consulta o crt.sh (só SANs ao vivo) |
| `--no-live` | não faz handshake ao vivo (só CT) |
| `--resolve` | resolve DNS e marca os vivos (**padrão: ligado**) |
| `--no-resolve` | desliga a resolução DNS (vence `--resolve`) |
| `--port=N` | porta do handshake quando o host não traz porta (padrão: 443) |
| `--concurrency=N` | resoluções/handshakes em paralelo, 1..64 (padrão: 8) |
| `--timeout=S` | timeout por request/conexão, em segundos (padrão: 20) |
| `--out-txt=PATH` | lista, um domínio por linha (padrão: `certspy-<ts>.txt`) |
| `--out-jsonl=PATH` | metadados por domínio (padrão: `certspy-<ts>.jsonl`) |
| `--out-md=PATH` | relatório markdown (padrão: `certspy-<ts>.md`) |
| `--no-md` | não gera o markdown |
| `--publish-url=URL` | **opcional**: envia o resultado por POST (JSON) ao seu endpoint |
| `--publish-token=STR` | valor do header `Authorization` no publish |
| `--publish-all` | inclui no publish nomes **fora** do apex do alvo |
| `--insecure` | não valida TLS no handshake ao vivo e no publish |
| `--user-agent=UA` | User-Agent das requests HTTP (padrão identificável) |
| `--quiet` | não imprime o progresso por domínio (avisos de falha continuam) |

Precisa de pelo menos uma fonte com entrada: `--domain` alimenta o CT e
`--host` alimenta o handshake ao vivo. `--no-ct` junto com `--no-live` é
recusado.

## Fontes

- **Certificate Transparency (`ct`)** — logs públicos e abertos onde as
  autoridades certificadoras registram cada certificado emitido. O certspy
  consulta o **crt.sh**, um serviço **de terceiros** (Sectigo), com
  `GET https://crt.sh/?q=%25.<dominio>&output=json` e também sem o `%25.`
  para o apex. O campo `name_value` traz vários nomes por certificado,
  separados por `\n`; o certspy quebra, tira o `*.` de wildcards, baixa a
  caixa e deduplica. O consumo é de dado **público**, mas a consulta
  revela ao crt.sh qual domínio você está investigando.
- **SANs ao vivo (`live`)** — handshake TLS no `--host` e leitura de
  `DNSNames` + CN do certificado apresentado. Usa SNI. Sem `--insecure`, um
  certificado inválido (vencido, autoassinado) derruba o handshake e nada é
  coletado; com `--insecure` o certificado é lido sem validar a cadeia.

Cada domínio guarda de quais fontes veio e o `not_before` mais antigo visto
no CT (a primeira vez que o nome apareceu num certificado).

## Saídas

- `.txt` — um domínio por linha, ordenado.
- `.jsonl` — um objeto por linha:

```json
{"nome":"api.meusite.com.br","fontes":["ct","live"],"not_before":"2023-07-15T08:30:00","vivo":true,"ips":["203.0.113.10"],"sob_apex":true}
```

- `.md` — cabeçalho com alvo e data, resumo (total, vivos, por fonte) e
  tabela `domínio | vivo | fontes | IPs`.

Os arquivos locais trazem **tudo** que foi descoberto; `sob_apex` marca o
que pertence ao alvo. Certificados compartilhados trazem às vezes nomes de
terceiros — eles aparecem no relatório local, mas não saem na publicação.

## Publicação externa

**Desligada por padrão.** Nada é enviado a lugar nenhum a menos que você
passe `--publish-url`, e o destino é **o endpoint que você fornece** — o
certspy não embute nenhum.

```bash
certspy --domain=meusite.com.br \
  --publish-url=https://meu-receptor.exemplo.com/certspy \
  --publish-token="Bearer ..."
```

- Um banner avisa **antes** da varredura para onde vai publicar e com que
  escopo.
- O corpo é `{"target":[...], "generated_at":"...", "domains":[...]}` com
  `Content-Type: application/json`.
- **Por padrão só vão nomes sob o apex do alvo.** `--publish-all` inclui os
  de fora, por sua conta e risco.
- O POST roda depois das saídas locais gravadas: se falhar, `.txt`, `.jsonl`
  e `.md` continuam lá.

## Uso responsável

Use para mapear a superfície do **seu** serviço ou de um alvo que você tem
**autorização** para testar. Os dados do crt.sh são públicos, mas isso não
autoriza nada além de olhar; o que você faz com a lista (por exemplo,
varrer cada host) precisa de autorização própria. Não publique nomes nem
dados pessoais de terceiros — é por isso que a publicação filtra pelo apex
do alvo por padrão. Respeite a LGPD e os termos do crt.sh.

## Limitações conhecidas

- **O crt.sh pode estar lento ou instável** e devolver HTML ou resposta
  vazia sob carga. O certspy trata como falha da consulta (aviso em stderr)
  e segue com as demais fontes; tente de novo ou aumente `--timeout`.
- **O CT lista histórico.** Nem todo nome está no ar; por isso o
  `--resolve` marca os que têm A/AAAA. "Vivo" significa só que o nome
  resolve, não que o serviço responde.
- **SANs ao vivo dependem do host responder** TLS na porta indicada.
- **Sem `--domain`, o apex é aproximado** pelos dois últimos rótulos do
  `--host`; sufixos como `co.uk` não são cobertos.
- **Não faz brute force de DNS.** Só encontra o que algum certificado
  público já citou.
- Resolve também os nomes fora do apex (consulta DNS pública de nomes de
  terceiros); use `--no-resolve` se isso for um problema.

## Desenvolvimento

```bash
go test ./...      # suíte de testes (não usa rede)
go vet ./...       # análise estática
go build -o certspy .
```

## Licença

[MIT](LICENSE).
