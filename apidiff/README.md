# apidiff

Comparador de scans de linha de comando, em Go. Você passa **dois scans**
que já gerou (do `download-dump` ou do `spyder`) e ele mostra **o que
mudou**: o drift da superfície de ataque entre duas rodadas — o que surgiu,
o que sumiu e o que mudou de status/conteúdo. **Não toca a rede.**

```
scan antigo (.jsonl | .db) ─┐
                            ├─►  normaliza  ─►  ADICIONADOS  (só no novo)
scan novo   (.jsonl | .db) ─┘    (chave+campos)  REMOVIDOS    (só no velho)
                                                 ALTERADOS    (status/kind/sha256: antes -> depois)
```

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/apidiff@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/apidiff
cd apidiff
go build -o apidiff .
```

## Uso rápido

```bash
apidiff --old=download-dump-20260101-100000.jsonl --new=download-dump-20260201-100000.jsonl
```

Saída de exemplo:

```
ADICIONADOS (1)
  + https://meusite.com.br/admin/novo.js  [200]

ALTERADOS (1)
  ~ https://meusite.com.br/app.js  sha256: 9f2c... -> a41b...

resumo: 1 adicionados, 0 removidos, 1 alterados
```

## Flags

| Flag | O que faz |
|---|---|
| `--old=PATH` | **(obrigatório)** scan antigo: `.jsonl` (download-dump) ou `.db` (spyder) |
| `--new=PATH` | **(obrigatório)** scan novo, no mesmo formato |
| `--format=FMT` | saída: `text` (padrão) ou `jsonl` |
| `--only=LIST` | filtra tipos de mudança, csv: `added,removed,changed` (padrão: todos) |
| `--log-file=PATH` | com `--format=jsonl`, grava o resultado também neste arquivo |
| `--quiet` | no modo `text`, imprime só o resumo |

## Formatos de entrada

O formato é detectado pela **extensão** do arquivo.

| Extensão | Origem | Chave | Campos comparados |
|---|---|---|---|
| `.jsonl` | manifesto do `download-dump` | `url` | `status`, `sha256` |
| `.db` | banco SQLite do `spyder` (tabela `routes`) | `url` + método | `status`, `kind` |

No `.jsonl`, linhas marcadas como `skipped`, `sensitive` ou `error` são
ignoradas: não têm conteúdo baixado, então comparar o hash delas geraria
"alteração" falsa. No `.db`, a mesma URL com `GET` e `POST` são rotas
distintas.

## Exemplos

Comparar dois manifestos do `download-dump`:

```bash
apidiff --old=download-dump-20260101-100000.jsonl --new=download-dump-20260201-100000.jsonl
```

Comparar dois bancos do `spyder`, só o que apareceu de novo:

```bash
apidiff --old=spyder-20260101.db --new=spyder-20260201.db --only=added
```

Saída em JSONL (um objeto por mudança), gravando também em arquivo:

```bash
apidiff --old=a.jsonl --new=b.jsonl --format=jsonl --log-file=apidiff-ts.jsonl
```

```json
{"change":"added","url":"https://site.com/novo.js","status":200,"sha256":"..."}
{"change":"changed","url":"https://site.com/app.js","field":"status","from":"200","to":"403"}
```

## Uso responsável

O `apidiff` opera **só sobre artefatos que você já gerou** (manifestos e
bancos de scans anteriores). Ele não faz nenhuma requisição e não toca a
rede. Os scans de origem, porém, só devem ter sido feitos em serviços seus
ou com autorização explícita — e carregam URLs e hashes do alvo, então
guarde-os como evidência sensível.

## Limitações conhecidas

- **`.jsonl` casa por URL; `.db` casa por URL + método.** Comparar um `.jsonl`
  com um `.db` não faz sentido: as chaves diferem e tudo aparece como
  adicionado/removido.
- **O hash só existe no `.jsonl`** (e o `kind` só no `.db`); cada formato
  compara os campos que tem.
- **Não faz diff de corpo.** Detecta *que* o sha256 mudou, não *o que* mudou
  dentro do arquivo.
- **Não ordena por relevância.** A saída é ordenada por chave; um `.js` de
  analytics mudando pesa o mesmo que um endpoint de API.

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o apidiff .
```

## Licença

[MIT](LICENSE).
