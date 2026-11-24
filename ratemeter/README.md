# ratemeter

Medidor de **rate-limit/quota** de linha de comando, em Go. Você passa o
endpoint do **seu** serviço e ele envia uma sequência **lenta e controlada**
de requests idênticos, registrando status, latência, o **primeiro 429**, o
`Retry-After` e os headers de rate-limit (`X-RateLimit-*` e `RateLimit-*`).
Por padrão **para no primeiro 429**.

```
VOCÊ PASSA:   --url=https://api.meusite.com/v1/x --rate=5 --ramp
                     │  rampa controlada (1/s -> 5/s), tetos rígidos
                     ▼
REGISTRA:     status · latência · 429 · Retry-After · X-RateLimit-* / RateLimit-*
                     │  para no primeiro 429 (padrão)
                     ▼
SAÍDA:        resumo no console   ← histograma de status, primeiro 429, p50/p95, limites vistos
              ratemeter-<ts>.jsonl ← uma linha por request
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/ratemeter@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/ratemeter
cd ratemeter
go build -o ratemeter .
```

## Uso rápido

```bash
ratemeter --url=https://api.meusite.com.br/v1/consulta \
  --header="Authorization: Bearer ..." --rate=5 --ramp
```

Sobe de 1 req/s até 5 req/s em 5 degraus, divididos igualmente entre os
`--total` (padrão 60) requests, e para no primeiro 429. O resumo diz em que
request e a quantos segundos o limite estourou.

## Flags

| Flag | O que faz |
|---|---|
| `--url` | **(obrigatório)** endpoint medido, com scheme (`http://` ou `https://`) |
| `--method=MÉTODO` | método HTTP (padrão: `GET`) |
| `--header="N: V"` | header de request, **repetível** |
| `--body=STR` | corpo do request (POST/PUT) |
| `--body-file=PATH` | corpo lido de arquivo (exclusivo com `--body`) |
| `--rate=N` | requests por segundo alvo (padrão: 1). **TETO RÍGIDO: 50** |
| `--ramp` | sobe de 1/s até `--rate` em degraus |
| `--ramp-steps=N` | nº de degraus da rampa (padrão: 5) |
| `--total=N` | total de requests (padrão: 60). **TETO RÍGIDO: 5000** |
| `--duration=S` | para após S segundos; `0` = só `--total` manda. O teto do total vale sempre |
| `--concurrency=N` | requests em voo em paralelo (padrão: 1). **TETO RÍGIDO: 20** |
| `--stop-on-429` | para assim que receber 429 (**PADRÃO LIGADO**) |
| `--continue-on-429` | desliga o stop-on-429 (só no **seu** alvo, com cautela) |
| `--timeout=S` | timeout por request (padrão: 15) |
| `--insecure` | não valida o cert TLS do destino |
| `--log-file=PATH` | JSONL por request (padrão: `ratemeter-<ts>.jsonl`) |
| `--quiet` | não imprime uma linha por request |

Valores acima dos tetos são **recusados** antes de qualquer request, com
mensagem clara (`--total acima do teto de segurança 5000: 6000`). Não existe
flag para subir os tetos.

## Log (JSONL)

Um objeto por request:

```json
{"ts":"2026-01-01T12:00:00Z","seq":12,"status":429,"duration_ms":38,"elapsed_ms":11002,
 "retry_after":"30","ratelimit_limit":"10","ratelimit_remaining":"0","ratelimit_reset":"30"}
```

`status` `0` com `error` significa que não houve resposta (rede/timeout).

## Por que não é DoS

- **Conservador por padrão:** 1 req/s, 60 requests, 1 em voo por vez.
- **Tetos rígidos:** no máximo 50 req/s, 5000 requests e 20 em paralelo,
  recusados na validação. Mesmo `--duration` não fura o teto do total.
- **Para no 429:** o primeiro sinal de limite encerra o teste. Continuar
  exige `--continue-on-429`, de forma explícita.
- **Requests idênticos e sem corpo de resposta guardado:** só status e
  headers interessam.

## Uso responsável

O `ratemeter` faz **requisições ativas** contra o alvo. Use **só** em
serviço que é seu ou que você tem **autorização explícita** para testar.
Mantenha `--rate` e `--concurrency` civis: o objetivo é descobrir onde o
limite está, não derrubar o serviço. Respeite os Termos de Uso e a lei
aplicável.

## Limitações conhecidas

- **Mede o que o alvo expõe em headers.** Se o servidor não manda
  `X-RateLimit-*`/`RateLimit-*`, a ferramenta não infere a quota; só vê o
  429 quando ele acontece.
- **Rede e latência influem:** o ritmo real pode ficar abaixo do alvo se o
  endpoint responder mais devagar que o intervalo e a concorrência estiver
  baixa.
- **Janela do limite desconhecida:** sem `Reset`/`Retry-After`, o tempo até
  o 429 é só uma pista da janela, não uma medida dela.
- **Não segue redirects:** o status medido é o do endpoint informado.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o ratemeter .
```

## Licença

[MIT](LICENSE).
