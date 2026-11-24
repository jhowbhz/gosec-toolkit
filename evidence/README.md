# evidence

Capstone do pentest, em Python 3 (só biblioteca padrão). Ele **não coleta
nada novo**: lê os artefatos que as outras ferramentas já gravaram em disco
(manifestos do `download-dump`, bancos do `spyder`, domínios do `certspy`,
achados do `jsleak`, `headeraudit`, `tlsscan` e `ratemeter`) e monta **um
único relatório Markdown de evidências**, com um passe de **redação de segredos**.

```
ARTEFATOS:    download-dump-*.jsonl  spyder-*.db  jsleak-*.jsonl  certspy-*.jsonl
              headeraudit-*.jsonl    tlsscan-*.jsonl  ratemeter-*.jsonl
                     │  classifica (nome na descoberta, chaves da 1ª linha no tipo)
                     ▼
REDAÇÃO:      AKIA… / AIza… / ghp_… / JWT / api_key=… → "AKIA…[20 chars]"
                     │
                     ▼
RELATÓRIO:    evidence-<ts>.md   ← resumo, superfície, sensíveis, segredos,
                                   postura HTTP, TLS, rate-limit, apêndice
```

## Instalação

Só precisa de Python 3.11+ (testado no 3.12). Nada de `pip install`:

```bash
git clone https://github.com/jhowbhz/pentest-projeto-nubrasil
cd pentest-projeto-nubrasil/tools/evidence
python evidence.py --help
```

## Uso rápido

```bash
python evidence.py --project-dir ..
```

Varre o diretório, acha os artefatos pelo padrão de nome, classifica e grava
`evidence-<timestamp>.md`. Com caminhos explícitos (a varredura é desligada):

```bash
python evidence.py --manifest download-dump-1.jsonl --spyder-db spyder-1.db \
  --findings jsleak-1.jsonl --findings headeraudit-1.jsonl --target "meusite.com.br"
```

Se nada for encontrado ou passado, ele sai com erro e mensagem clara — não
gera relatório vazio.

## Flags

| Flag | O que faz |
|---|---|
| `--project-dir PATH` | diretório a varrer por artefatos (padrão: `.`) |
| `--manifest PATH` | manifesto `download-dump` explícito, **repetível** |
| `--spyder-db PATH` | banco `spyder` explícito, **repetível** |
| `--findings PATH` | `.jsonl` de achados explícito (qualquer ferramenta), **repetível** |
| `--target NAME` | rótulo do alvo no cabeçalho (padrão: título do `README.md` do projeto; senão `alvo autorizado`) |
| `--out PATH` | arquivo `.md` de saída (padrão: `evidence-<ts>.md`) |
| `--no-redact` | mantém os segredos crus (padrão: **redige**) |
| `--quiet` | não imprime mensagens de progresso |

## Classificação dos `.jsonl`

Pela 1ª linha do arquivo:

| Chaves | Ferramenta |
|---|---|
| `rule` + `severity` | jsleak |
| `versoes_aceitas` ou `certs` | tlsscan |
| `seq` + `retry_after` | ratemeter |
| `nome` + `fontes` | certspy |
| `sha256`, `saved_path`, `sensitive` ou `skip_reason` | download-dump |
| `grade` ou `url` + `issues` | headeraudit |

## Redação

A função `redigir()` mascara, **em todo texto que entra no relatório**, a
menos que `--no-redact`:

- chaves AWS (`AKIA…`), Google (`AIza…`), GitHub (`ghp_…`, `gho_…`…);
- JWT (`eyJ….eyJ….assinatura`);
- pares `api_key=`, `secret=`, `password=`, `token=` (mantém a chave, esconde o valor);
- strings longas (32+ caracteres, com letras e dígitos) com cara de token.

O resultado mostra um prefixo curto e o tamanho: `AKIA…[20 chars]`. A
redação é reaplicada mesmo sobre achados do `jsleak`, que já podem vir
redigidos — o `evidence` nunca confia na etapa anterior.

## Uso responsável

O relatório documenta um teste **autorizado**: só gere para serviços seus ou
com autorização explícita. Ele redige segredos por padrão; use `--no-redact`
apenas num ambiente que você controla e nunca o publique. Não divulgue PII de
terceiros que aparecer nas evidências (LGPD).

## Limitações conhecidas

- **Depende dos artefatos existirem.** Sem os `.jsonl`/`.db` das outras
  ferramentas, não há o que relatar.
- **Classificação por chave é heurística.** Um `.jsonl` de formato novo ou
  alterado pode cair em "não reconhecido" (o arquivo é ignorado com aviso).
- **A redação é best-effort.** Segredos fora dos padrões acima podem passar;
  revise o relatório antes de compartilhar.
- O formato do `ratemeter` é lido de forma tolerante (`url`, `seq`, `status`,
  `retry_after` e qualquer chave com `limit`).

## Desenvolvimento

```bash
python -m unittest -v      # suíte de testes
python evidence.py --help
```

## Licença

[MIT](LICENSE).
