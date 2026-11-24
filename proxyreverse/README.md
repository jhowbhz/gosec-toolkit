# proxyreverse

Proxy reverso de linha de comando, em Go. Dá um **nome bonito** pro seu
serviço local: você acessa `http://meuapp.local/` e ele repassa pra onde o
serviço realmente roda (`http://127.0.0.1:3000`, por exemplo) — registrando
cada request e response num log pra você inspecionar.

```
VOCÊ ABRE:   http://meuapp.local/        ← o --from (o proxy escuta aqui)
                     │  repassa
                     ▼
--to:        http://127.0.0.1:3000       ← seu serviço real, que PRECISA estar no ar
```

> Você sempre abre o **`--from`** no navegador. O **`--to`** é só pra onde o
> proxy repassa por baixo — não se acessa o `--to` direto.

## Instalação

Com Go instalado (1.27+):

```bash
go install github.com/jhowbhz/proxyreverse@latest
```

Ou compile do código:

```bash
git clone https://github.com/jhowbhz/proxyreverse
cd proxyreverse
go build -o proxyreverse .
```

## Uso rápido (dev local)

Digamos que seu app roda em `http://127.0.0.1:3000` e você quer acessá-lo por
`http://meuapp.local/`:

1. Aponte o nome pra você no `hosts`
   (`C:\Windows\System32\drivers\etc\hosts` no Windows, `/etc/hosts` no Linux/Mac):
   ```
   127.0.0.1   meuapp.local
   ```
2. Suba o proxy (porta 80 pede terminal **admin/root**; veja `--http` pra porta alta):
   ```bash
   proxyreverse --from=meuapp.local --to=http://127.0.0.1:3000
   ```
3. Abra **`http://meuapp.local/`** no navegador.

> **Importante:** o `--to` tem que estar rodando. Se der
> `connection refused` / `recusou ativamente`, é porque não há serviço naquela
> porta — suba seu app primeiro, ou confira a porta certa.

Sem precisar de admin, use uma porta alta:

```bash
proxyreverse --from=meuapp.local --to=http://127.0.0.1:3000 --http=127.0.0.1:8080
# abra http://meuapp.local:8080/
```

## Flags

| Flag | O que faz |
|---|---|
| `--from` | **(obrigatório)** nome que o proxy atende — o mesmo que você aponta pra `127.0.0.1` no `hosts` |
| `--to` | **(obrigatório)** destino real, **com scheme** (`http://` ou `https://`) |
| `--http=ADDR` | escuta HTTP (ex.: `127.0.0.1:8080` ou `:80`) |
| `--https=ADDR` | escuta HTTPS (ex.: `127.0.0.1:8443` ou `:443`) |
| | sem `--http` nem `--https`: sobe os dois em `:80` e `:443` |
| `--upstream-addr=HOST:PORTA` | disca esse IP/porta real, mantendo `Host`/SNI do `--to`; use quando o nome aponta pra `127.0.0.1` no `hosts` |
| `--header "Nome: valor"` | header extra enviado ao destino (repetível) |
| `--preserve-host` | mantém o `Host` original no request pro upstream |
| `--insecure` | não valida o cert TLS do upstream (dev / self-signed) |
| `--cert` / `--key` | cert e chave próprios pra escuta HTTPS (senão: auto-assinado na hora) |
| `--log-file=PATH` | caminho do log JSONL (padrão: `proxyreverse-<timestamp>.jsonl`) |
| `--max-body=KB` | teto de corpo logado, em KB (padrão: 256) |
| `--quiet` | não imprime o resumo por request no console |

## HTTPS local

Com `--https` (ou nos padrões), o proxy gera um **certificado auto-assinado** na
hora pro `--from`. O navegador vai avisar que não confia — é esperado; confie
manualmente, ou passe `--cert`/`--key` com um cert do
[`mkcert`](https://github.com/FiloSottile/mkcert).

## Log das trocas

Um objeto JSON por linha (JSONL), com headers e corpo dos dois lados:

```json
{"ts":"2026-11-24T00:38:16Z","id":"fee8ec1f6672","method":"POST",
 "url":"http://meuapp.local/api/login","req_headers":{...},"req_body":"...",
 "status":200,"resp_headers":{...},"resp_body":"...","duration_ms":12}
```

`req_headers` são os headers **realmente enviados ao destino** (já com o
`X-Proxyreverse`, os `--header` e os `X-Forwarded-*`) — e não os que o cliente
mandou. Num proxy de interceptação é isso que interessa inspecionar.

Falha de upstream (destino fora do ar) também vira registro, com `status:0` e o
campo `error` — e o proxy devolve **502** pro cliente.

## Direção do Host

Por padrão o header `Host` é **reescrito pro destino** (`--to`). Use
`--preserve-host` pra manter o nome original — necessário quando o destino roteia
por `Host`/vhost (ex.: um serviço atrás de um CDN que você acessa pelo IP real).

## Interceptar TODO o tráfego de um site (nome → proxy)

Quando você quer que o navegador acesse `site.com` normalmente e **tudo** passe
pelo proxy (página, assets, chamadas de API da mesma origem), aponte o nome pra
você no `hosts` e deixe o proxy nas portas padrão:

```
# hosts:  127.0.0.1   site.com
```

Mas aí tem um nó: com o nome apontando pra `127.0.0.1`, um `--to=https://site.com`
resolveria de volta pra você (loop). A flag **`--upstream-addr`** resolve isso —
ela disca o **IP real** da origem, mas mantém o `Host` e o SNI vindos do `--to`:

```bash
# pegue o IP real ANTES de editar o hosts:  nslookup site.com
proxyreverse --from=site.com --to=https://site.com --upstream-addr=<IP_REAL>:443
# terminal admin (portas 80/443)
```

Fluxo: navegador → `https://site.com` (hosts → 127.0.0.1) → proxy em :443 →
disca `<IP_REAL>:443` com `Host`/SNI = `site.com` → a origem (mesmo atrás de CDN)
responde certo. Tudo que a página pede na mesma origem continua passando pelo
proxy e caindo no log.

> Origens **diferentes** (ex.: um `api.outrodominio.com` que a página chama) só
> passam pelo proxy se você também apontá-las no `hosts` e rodar uma instância
> pra cada uma.

## Headers pro destino

Toda request encaminhada leva um header **`X-Proxyreverse: proxyreverse/<versão>`**
que identifica a ferramenta no destino — enviado **sempre**, mesmo sem configurar
nada (útil em pentest autorizado, onde o destino espera/reconhece o tráfego de teste).

Pra adicionar outros headers (auth, marcador combinado com o time, etc.), use
`--header` quantas vezes quiser. Um `--header` com o mesmo nome **sobrescreve** o
padrão:

```bash
proxyreverse --from=site.com --to=https://site.com --upstream-addr=<IP>:443 \
  --header "X-Pentest: autorizado" \
  --header "Authorization: Bearer <token>"
```

Os headers que o navegador já manda continuam sendo repassados normalmente.

## Proteção contra loop

Apontar `--to` pro próprio proxy derrubaria a máquina (amplificação até esgotar
as portas). A ferramenta se defende em duas camadas:

- **No startup:** recusa se `--to` for loopback (`127.0.0.1`/`localhost`) na
  mesma porta que você está escutando.
- **Em cada request:** marca o que repassa com o header `X-Proxyreverse-Loop`; se
  um request chega já marcado, responde **508** e não repassa.

## Rodar em background

**Windows (PowerShell):**
```powershell
Start-Process -FilePath .\proxyreverse.exe `
  -ArgumentList '--from=meuapp.local','--to=http://127.0.0.1:3000' -WindowStyle Hidden
```
**Linux/Mac:**
```bash
proxyreverse --from=meuapp.local --to=http://127.0.0.1:3000 &
```
No foreground, `Ctrl+C` encerra graciosamente.

## Uso responsável

`proxyreverse` fica no meio do tráfego e registra tudo que passa. Use **só** em
serviços que são seus ou que você tem **autorização explícita** para testar. É
uma ferramenta de desenvolvimento e estudo — não a use pra interceptar tráfego
de terceiros.

## Limitações conhecidas

- **Bufferiza o corpo** de request e response em memória pra poder registrar — é
  um proxy de inspeção, não serve pra streaming de uploads/downloads gigantes
  nem SSE contínuo. O corpo logado é cortado em `--max-body`; o repasse é inteiro.
- O cert auto-assinado **não é confiável** por padrão (esperado).

## Desenvolvimento

```bash
go test ./...      # suíte de testes
go vet ./...       # análise estática
go build -o proxyreverse .
```

## Licença

[MIT](LICENSE).
