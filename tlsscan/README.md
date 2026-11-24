# tlsscan

Inspetor de TLS de linha de comando, em Go (só biblioteca padrão). Você passa
um `host[:porta]` e ele descobre **quais versões de TLS o servidor aceita**,
**qual cipher negocia em cada uma**, lê o **certificado** (e, se pedir, a
cadeia inteira), confere o **HSTS** e devolve uma **lista de fraquezas** com
uma **nota de A a F**. É passivo: só handshakes e um `GET`.

```
VOCÊ PASSA:   --host=meusite.com.br
                     │
        ┌────────────┼──────────────────────┐
        ▼            ▼                      ▼
  handshake por   cadeia de certs      GET https://host/
  versão fixa     (folha ou toda)      lê Strict-Transport-Security
  TLS1.0..1.3     CN/SAN, validade,
  + cipher        assinatura, chave
        └────────────┼──────────────────────┘
                     ▼
              issues (severidade)  ->  nota A..F
                     ▼
RELATÓRIO:    tlsscan-<ts>.jsonl     (um objeto por host)
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/tlsscan@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/tlsscan
cd tlsscan
go build -o tlsscan .
```

## Uso rápido

```bash
tlsscan --host=meusite.com.br
tlsscan --host=meusite.com.br --host=api.meusite.com.br:8443 --full-chain
tlsscan --hosts-file=alvos.txt --quiet
```

## Flags

| Flag | O que faz |
|---|---|
| `--host=HOST[:PORT]` | host a escanear, **repetível** |
| `--hosts-file=PATH` | arquivo com um host por linha (`#` comenta) |
| `--port=N` | porta usada quando o host não traz porta (padrão: 443) |
| `--servername=SNI` | força o SNI (padrão: o próprio host) |
| `--timeout=S` | timeout por conexão, em segundos (padrão: 15) |
| `--full-chain` | mostra todos os certs da cadeia (padrão: só a folha) |
| `--no-hsts` | não faz o `GET` de checagem de HSTS (só handshake) |
| `--log-file=PATH` | relatório JSONL (padrão: `tlsscan-<ts>.jsonl`) |
| `--quiet` | não imprime o resumo por host no console |

É obrigatório informar `--host` ou `--hosts-file`.

## Versões e ciphers

Para cada versão (TLS 1.0, 1.1, 1.2, 1.3) a ferramenta tenta um handshake
com `MinVersion = MaxVersion = versão`. Se completa, a versão é **aceita** e
o cipher negociado entra em `ciphers`. Se o servidor aceita só TLS 1.2 e 1.3,
você vê duas entradas; se aceita TLS 1.0, aparece o alerta.

SSLv3 não é testado: o `crypto/tls` do Go não o implementa.

## O que cada issue significa

| Check | Severidade | Significado |
|---|---|---|
| `protocolo-obsoleto` | high | o servidor aceita TLS 1.0 ou 1.1, depreciados (RFC 8996) |
| `cert-expirado` | critical | o certificado já passou do `NotAfter`; clientes recusam |
| `cert-expira-logo` | high / medium | faltam menos de 15 dias (high) ou 30 dias (medium) |
| `cert-ainda-invalido` | high | o relógio está antes do `NotBefore` |
| `assinatura-fraca` | high | assinado com SHA-1, MD5 ou MD2, quebráveis por colisão |
| `rsa-curta` | high | chave RSA abaixo de 2048 bits |
| `auto-assinado` | medium | emissor e sujeito são o mesmo e a assinatura fecha com a própria chave; se for intencional (ambiente interno), leia como info |
| `sem-hsts` | medium | a resposta https não traz `Strict-Transport-Security` |

A **nota** sai das issues: qualquer `critical` = **F**; duas ou mais `high` =
**E**; uma `high` = **D**; duas ou mais `medium` = **C**; uma `medium` =
**B**; nenhuma = **A**.

`verificacao_ok` / `verificacao_erro` dizem se a cadeia seria confiável para
o SNI **a partir desta máquina**. Ficam à parte das issues porque uma raiz
interna que só falta aqui não é fraqueza do servidor.

## Relatório (JSONL)

```json
{"ts":"...","host":"meusite.com.br:443",
 "versoes_aceitas":["TLS 1.2","TLS 1.3"],
 "ciphers":{"TLS 1.2":"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS 1.3":"TLS_AES_128_GCM_SHA256"},
 "certs":[{"subject":"meusite.com.br","emissor":"R11","not_after":"2027-01-10","dias_para_expirar":48,"assinatura":"SHA256-RSA","chave":"RSA 2048","auto_assinado":false}],
 "verificacao_ok":true,"hsts":"max-age=31536000","grade":"A","issues":[]}
```

Host que não completa nenhum handshake vira uma linha com `erro` e `grade: "-"`.

## Uso responsável

O `tlsscan` é passivo (handshakes e um `GET` por host), mas ainda é tráfego
contra o alvo. Escaneie **só** hosts que são seus ou que você tem
**autorização explícita** para testar.

## Limitações conhecidas

- **Não enumera todos os ciphers do servidor.** O `crypto/tls` do Go só
  informa o cipher **negociado** em cada versão, não a lista suportada.
- **TLS 1.3 não permite fixar cipher** no Go; sai o que o Go preferir.
- **Não cobre cadeia incompleta nem OCSP:** só relata o que o servidor envia
  e o resultado de `Verify` com esses certs.
- **Sem SSLv3** (não implementado no Go).
- Escaneia os hosts em série, um de cada vez.

## Desenvolvimento

```bash
go test ./...      # suíte de testes (certs gerados em memória, sem rede)
go vet ./...       # análise estática
go build -o tlsscan .
```

## Licença

[MIT](LICENSE).
