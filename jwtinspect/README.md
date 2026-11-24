# jwtinspect

Inspetor de JWT de linha de comando, em Go. Você passa **os seus próprios
tokens** de sessão e ele **decodifica e audita**: mostra `alg`, `kid` e os
claims principais, confere a janela de validade e sinaliza configurações
perigosas. Tudo **offline** — não faz nenhuma requisição.

```
VOCÊ PASSA:   --token / --token-file / stdin
                     │  decodifica header e payload (base64url)
                     ▼
CLAIMS:       alg, kid, iss, sub, aud, exp, iat, nbf
                     │  checagens: alg, janela de validade, vida útil
                     ▼
ACHADOS:      issues[] (critical / medium / low / info)
                     │
                     ▼
RELATÓRIO:    jwtinspect-<ts>.jsonl      ← um objeto por token

OPCIONAL:     --secret / --secrets
                     │  recalcula o HMAC do SEU token com cada candidato
                     ▼
              "SEGREDO FRACO: '<cand>' valida a assinatura"   (critical)
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/jwtinspect@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/jwtinspect
cd jwtinspect
go build -o jwtinspect .
```

## Uso rápido

```bash
jwtinspect --token=eyJhbGciOiJIUzI1NiJ9...
```

Vários tokens, um por linha, e a checagem de segredo fraco:

```bash
jwtinspect --token-file=sessoes.txt --secrets=senhas-comuns.txt
```

Pelo pipe:

```bash
cat sessoes.txt | jwtinspect --quiet --log-file=auditoria.jsonl
```

## Flags

| Flag | O que faz |
|---|---|
| `--token=JWT` | token a inspecionar, **repetível** |
| `--token-file=PATH` | arquivo com um token por linha |
| *(nenhum dos dois)* | lê os tokens do stdin, um por linha |
| `--secret=STR` | candidato de segredo HMAC a testar offline, **repetível** |
| `--secrets=PATH` | wordlist de candidatos de segredo, um por linha |
| `--leeway=S` | folga de relógio, em segundos, para `exp`/`nbf`/`iat` (padrão: 60) |
| `--max-life=H` | acima disso (`exp - iat`) o token é "vida longa", em horas; `0` desliga (padrão: 24) |
| `--log-file=PATH` | relatório JSONL (padrão: `jwtinspect-<ts>.jsonl`) |
| `--quiet` | não imprime o resultado por token no console |

## Sinais que ele levanta

| Check | Severidade | Significa |
|---|---|---|
| `alg_none` | critical | `alg: none` — o token não tem assinatura; se o servidor aceitar, qualquer um forja claims |
| segredo fraco | critical | um candidato de `--secret`/`--secrets` reproduz a assinatura HS* |
| `exp_ausente` | medium | sem `exp` — o token nunca expira sozinho |
| `vida_longa` | low / medium | `exp - iat` acima de `--max-life` (medium se passar de 30 dias) |
| `iat_futuro` | low | emitido no futuro além da folga — relógio desalinhado ou claim forjado |
| `expirado` | info | `exp` já passou (descontada a folga) |
| `nbf_futuro` | info | ainda não é válido (`nbf` no futuro, descontada a folga) |
| `alg_simetrico` | info | HS* onde se esperaria assimétrico: confirme que o verificador **fixa** o algoritmo (risco de alg-confusion) |

## Checagem de segredo: offline, sobre o SEU token

Com `--secret`/`--secrets`, a ferramenta recalcula o HMAC do **seu token**
com cada candidato e compara com a assinatura que já está nele. Isso é uma
conta local, feita na sua máquina, sobre um token que você já tem.

**Não é brute force de serviço remoto**: nenhuma tentativa é enviada a
servidor algum, não há login a forçar, e a ferramenta nem tem cliente de rede.
A pergunta que ela responde é defensiva: *"o segredo com que assino meus
tokens aparece numa lista de segredos comuns?"*. Se aparecer, troque o
segredo — quem tiver qualquer token seu consegue achá-lo da mesma forma.

## Relatório (JSONL)

Um objeto por token:

```json
{"ts":"...","token_id":"a1b2c3d4e5f6","alg":"HS256","kid":"k1","sub":"ana",
 "exp":"2033-05-18T03:33:20Z","validade":"valido",
 "issues":[{"check":"alg_simetrico","severity":"info","detalhe":"..."}],
 "weak_secret":"admin"}
```

O token **não** é gravado: `token_id` é o prefixo do seu SHA-256, só para
correlacionar. `weak_secret` aparece só quando um candidato validou. Token
malformado vira linha com `error`, sem interromper os demais.

## Uso responsável

Inspecione **os seus próprios tokens** — de sessões de serviços que são seus
ou que você tem autorização para testar. A lista de segredos serve para
avaliar a força do **seu** segredo de assinatura, não para descobrir o de
terceiros. Trate o relatório e os arquivos de tokens como credenciais.

## Limitações conhecidas

- **Só HS256/HS384/HS512 na verificação de segredo.** Em RS*/ES*/PS*/EdDSA
  não há segredo a adivinhar; a ferramenta **não valida a assinatura**
  RSA/EC (não recebe chave pública) — só decodifica e audita os claims.
- **Sem validação de assinatura em geral**: "valido" no relatório significa
  só que a janela `exp`/`nbf` está ok, não que o token seja autêntico.
- Só o formato compacto (JWS, 3 partes). JWE (5 partes) é recusado.
- `exp`/`nbf`/`iat` precisam ser números (NumericDate); outro tipo é tratado
  como ausente.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o jwtinspect .
```

## Licença

[MIT](LICENSE).
