# painel

Painel web, em Go (binário único), para rodar as ferramentas do kit pelo
navegador: você escolhe a ferramenta, preenche um formulário gerado do schema
dela, dispara, e acompanha a **saída ao vivo**. Cada execução guarda seus
artefatos (jsonl, espelho, relatórios) para baixar. Pensado para rodar **só na
sua máquina**.

```
navegador ──(token)──► painel (127.0.0.1:7777) ──exec(argv, sem shell)──► download-dump, tlsscan, ...
     ▲                      │                                                    │
     └── saída ao vivo (SSE) ┘                          artefatos em runs/<id>/ ─┘
```

## Segurança

O painel executa comandos — então ele é, por desenho, uma superfície de
execução. As defesas:

- **Só loopback.** Escuta em `127.0.0.1:7777`; ninguém de fora da máquina
  alcança. Escutar fora de loopback imprime aviso.
- **Token obrigatório.** Gerado no startup e embutido na URL que o terminal
  imprime. Toda chamada de API exige o header `X-Painel-Token` (o stream SSE,
  que não manda header, aceita `?token=`). A UI guarda o token no
  `sessionStorage` e o tira da barra de endereço. Outra aba/app local não
  conhece o token, então não dispara nada (barra CSRF/SSRF local).
- **Sem shell.** As ferramentas são executadas por `argv` (`exec.Command`),
  nunca por uma linha de shell — não há como injetar comando.
- **Schema fecha o que entra.** O painel só monta flags que estão no schema da
  ferramenta; qualquer flag fora dele é rejeitada. Campo `int`/`enum` é
  validado.
- **Saída confinada.** Cada execução roda com o diretório de trabalho em
  `runs/<id>/`; os arquivos que a ferramenta gera caem ali e viram artefatos. O
  download de artefato é restrito a essa pasta (barra path traversal).
- **Composição confinada.** Campos "run-ref" (uma ferramenta lendo o artefato
  de outra, ex.: `spyder-credentials` varrendo o espelho do `download-dump`) só
  aceitam caminhos **dentro** de `runs/`.

## Rodar

Na pasta `tools/painel/` (Go 1.27+):

```bash
go build -o painel .
./painel
```

O terminal imprime a URL com o token — abra no navegador:

```
painel no ar. Abra no navegador (o token já vai na URL):
  http://127.0.0.1:7777/?token=<token-gerado>
```

Opções (flags `--opcao=valor` ou variáveis de ambiente):

| Flag | Env | Padrão | O que faz |
|---|---|---|---|
| `--addr` | `PAINEL_ADDR` | `127.0.0.1:7777` | endereço de escuta |
| `--token` | `PAINEL_TOKEN` | gerado | token de acesso |
| `--tools-dir` | `PAINEL_TOOLS_DIR` | `..` | pasta com as ferramentas |

Os binários das ferramentas são resolvidos em `tools/<id>/`; se faltarem, o
painel os **compila** na primeira vez (`go build`). `evidence` roda via
`python`. `spyder` precisa de Chrome instalado.

## Como usar

1. Escolha a ferramenta na coluna da esquerda.
2. Preencha o formulário (campos obrigatórios marcados com `*`; campos
   repetíveis aceitam um valor por linha).
3. **Rodar** — a saída aparece ao vivo no console; **Parar** encerra um
   processo em andamento (útil para `proxyreverse`/`spyder`, que ficam no ar).
4. Em **Execuções**, clique num run para ver e baixar seus artefatos.

Campos "run-ref" listam artefatos de execuções anteriores, então dá para
encadear: rode o `download-dump`, depois escolha o espelho dele no
`spyder-credentials`.

## Ferramentas expostas

As 12 do kit: `download-dump`, `spyder-credentials`, `tlsscan`, `headeraudit`,
`jsleak`, `jwtinspect`, `ratemeter`, `certspy`, `apidiff`, `proxyreverse`,
`spyder`, `evidence`. O schema de cada uma fica em `registro_dados.go`.

## Uso responsável

O painel só facilita disparar as ferramentas — a responsabilidade de mirar
**alvo seu ou autorizado** continua sua, como nas CLIs. Rode local.

## Limitações conhecidas

- **Schema curado, não exaustivo.** Cada ferramenta expõe suas flags
  principais; flags raras ficam de fora do formulário (dá para adicionar em
  `registro_dados.go`).
- **Histórico em memória para o stream.** A saída ao vivo de um run fica em
  memória; reiniciar o painel perde o stream (mas o `output.log` de cada run
  continua em `runs/<id>/`).
- **Ferramentas longas** (`proxyreverse`, `spyder --forever`) seguem rodando
  até você clicar em Parar.

## Desenvolvimento

```bash
go test ./...      # suíte (registro, runs, runner, servidor, auth)
go vet ./...
go build -o painel .
```

## Licença

[MIT](LICENSE).
