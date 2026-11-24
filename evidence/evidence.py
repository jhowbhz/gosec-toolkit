#!/usr/bin/env python3
"""evidence: junta os artefatos das outras ferramentas num relatório Markdown.

É o capstone do pentest: não coleta nada novo, só lê o que download-dump,
spyder, certspy, jsleak, headeraudit, tlsscan e ratemeter já gravaram em disco,
classifica cada artefato, redige segredos e escreve UM relatório de evidências.
Apenas biblioteca padrão do Python.
"""

import argparse
import json
import os
import re
import sqlite3
import sys
from datetime import datetime
from pathlib import Path

# Tipos de artefato que o classificador sabe produzir.
TIPO_DOWNLOAD = "download-dump"
TIPO_SPYDER = "spyder"
TIPO_JSLEAK = "jsleak"
TIPO_HEADERAUDIT = "headeraudit"
TIPO_TLSSCAN = "tlsscan"
TIPO_RATEMETER = "ratemeter"
TIPO_CERTSPY = "certspy"

# Padrão de nome -> uso só na descoberta; o tipo real de um .jsonl vem das chaves.
PADROES_DESCOBERTA = ("download-dump-*.jsonl", "spyder-*.db", "jsleak-*.jsonl",
                      "headeraudit-*.jsonl", "tlsscan-*.jsonl", "ratemeter-*.jsonl",
                      "certspy-*.jsonl")

# Pastas que nunca contêm artefatos próprios e deixariam a varredura lenta.
PASTAS_IGNORADAS = {".git", "node_modules", "__pycache__", ".omc", "vendor"}

# Ordem do melhor para o pior; nota fora da lista conta como a pior conhecida.
ORDEM_NOTAS = ["A+", "A", "A-", "B", "C", "D", "E", "F"]
ORDEM_SEVERIDADE = ["critical", "critica", "crítica", "high", "alta", "medium",
                    "media", "média", "low", "baixa", "info"]
ORDEM_KINDS = ["page", "xhr", "asset", "external"]

# --- Redação ---------------------------------------------------------------

# Ordem importa: padrões específicos (com prefixo conhecido) antes do genérico,
# senão o heurístico de "string longa" engole o JWT inteiro sem preservar o formato.
_PADROES_SEGREDO = [
    re.compile(r"eyJ[A-Za-z0-9_-]{5,}\.eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*"),
    re.compile(r"\bAKIA[0-9A-Z]{12,}\b"),
    re.compile(r"\bAIza[0-9A-Za-z_-]{20,}"),
    re.compile(r"\bgh[pousr]_[A-Za-z0-9]{16,}"),
]
# Mantém a chave (grupo 1) e mascara só o valor (grupo 2): o leitor ainda vê
# QUAL segredo vazou, sem ver o segredo.
_PADRAO_CHAVE_VALOR = re.compile(
    r"""(?ix)
    ((?:api[_-]?key|apikey|secret|password|passwd|senha|token|access[_-]?key)
     ["']?\s*[:=]\s*["']?)
    ([^\s"'&,;]{4,})""")
# Token "de aparência": longo, sem '/', misturando letras e dígitos. Sem '/'
# para não mascarar caminhos de URL.
_PADRAO_TOKEN_LONGO = re.compile(r"(?<![A-Za-z0-9_+=-])[A-Za-z0-9_+=-]{32,}(?![A-Za-z0-9_+=-])")


def _mascarar(valor):
    """Mostra 4 caracteres de prefixo e o tamanho: dá para correlacionar sem expor."""
    return f"{valor[:4]}…[{len(valor)} chars]"


def _parece_token(candidato):
    """Exige letra E dígito: evita mascarar palavras longas e sequências só de números."""
    return bool(re.search(r"[A-Za-z]", candidato) and re.search(r"\d", candidato))


def redigir(texto):
    """Mascara segredos conhecidos e tokens de aparência em `texto`; função pura.

    Decide o que é segredo (chaves de nuvem, JWT, pares chave=valor, strings
    longas de token) e o substitui por prefixo + tamanho. Texto sem segredo
    volta idêntico.
    """
    if not texto:
        return texto
    for padrao in _PADROES_SEGREDO:
        texto = padrao.sub(lambda m: _mascarar(m.group(0)), texto)
    texto = _PADRAO_CHAVE_VALOR.sub(lambda m: m.group(1) + _mascarar(m.group(2)), texto)
    return _PADRAO_TOKEN_LONGO.sub(
        lambda m: _mascarar(m.group(0)) if _parece_token(m.group(0)) else m.group(0), texto)


# --- Leitura e classificação -----------------------------------------------

def classificar_jsonl(primeira_linha):
    """Decide qual ferramenta gerou um .jsonl olhando as chaves da 1ª linha.

    Heurística: as chaves mais exclusivas são testadas primeiro (tlsscan também
    tem "grade", então precisa vir antes de headeraudit). Devolve None se não
    reconhecer.
    """
    if not isinstance(primeira_linha, dict):
        return None
    chaves = set(primeira_linha)
    if {"rule", "severity"} <= chaves:
        return TIPO_JSLEAK
    if {"nome", "fontes"} <= chaves:
        return TIPO_CERTSPY
    if "versoes_aceitas" in chaves or "certs" in chaves:
        return TIPO_TLSSCAN
    if {"seq", "retry_after"} <= chaves:
        return TIPO_RATEMETER
    if chaves & {"sha256", "saved_path", "sensitive", "skip_reason"}:
        return TIPO_DOWNLOAD
    if "grade" in chaves or {"url", "issues"} <= chaves:
        return TIPO_HEADERAUDIT
    return None


def ler_jsonl(caminho):
    """Lê um .jsonl devolvendo só objetos válidos.

    Linhas quebradas são puladas (um crash no meio da gravação não deve
    inutilizar o resto da evidência).
    """
    registros = []
    with open(caminho, encoding="utf-8", errors="replace") as arquivo:
        for linha in arquivo:
            linha = linha.strip()
            if not linha:
                continue
            try:
                objeto = json.loads(linha)
            except json.JSONDecodeError:
                continue
            if isinstance(objeto, dict):
                registros.append(objeto)
    return registros


def ler_spyder_db(caminho):
    """Lê a tabela routes do banco do spyder como lista de dicts.

    Abre em modo somente leitura: evidência não pode ser alterada pelo relatório.
    """
    uri = Path(caminho).resolve().as_uri() + "?mode=ro"
    conexao = sqlite3.connect(uri, uri=True)
    try:
        conexao.row_factory = sqlite3.Row
        linhas = conexao.execute(
            "SELECT url, host, method, status, kind, source_url, depth, title, first_seen "
            "FROM routes ORDER BY url").fetchall()
        return [dict(linha) for linha in linhas]
    finally:
        conexao.close()


def descobrir_artefatos(diretorio):
    """Varre `diretorio` e devolve os caminhos que casam com os nomes conhecidos.

    Retorna {"jsonl": [...], "spyder": [...]}; a separação por ferramenta fica
    para a classificação por conteúdo, que não depende do nome do arquivo.
    """
    achados = {"jsonl": [], "spyder": []}
    raiz = Path(diretorio)
    for pasta, subpastas, arquivos in os.walk(raiz):
        subpastas[:] = [p for p in subpastas if p not in PASTAS_IGNORADAS]
        for nome in sorted(arquivos):
            caminho = Path(pasta) / nome
            if nome.startswith("spyder-") and nome.endswith(".db"):
                achados["spyder"].append(caminho)
            elif nome.endswith(".jsonl") and any(
                    Path(nome).match(p) for p in PADROES_DESCOBERTA if p.endswith(".jsonl")):
                achados["jsonl"].append(caminho)
    return achados


def descobrir_alvo(diretorio):
    """Usa o primeiro título do README.md do projeto como rótulo do alvo.

    Falha silenciosa de propósito: o rótulo é cosmético, não deve barrar o relatório.
    """
    try:
        with open(Path(diretorio) / "README.md", encoding="utf-8", errors="replace") as readme:
            for linha in readme:
                if linha.startswith("# "):
                    return linha[2:].strip() or "alvo autorizado"
    except OSError:
        pass
    return "alvo autorizado"


# --- Montagem do relatório -------------------------------------------------

def _posicao(valor, ordem):
    """Posição em `ordem`; o que não consta vai para o fim (tratado como o pior)."""
    minusculo = str(valor or "").lower()
    ordem_min = [o.lower() for o in ordem]
    return ordem_min.index(minusculo) if minusculo in ordem_min else len(ordem)


def _pior_nota(registros):
    """Pior nota de postura HTTP entre os registros, ou None se nenhuma tem nota."""
    notas = [r["grade"] for r in registros if r.get("grade")]
    return max(notas, key=lambda n: _posicao(n, ORDEM_NOTAS)) if notas else None


def _tabela(cabecalho, linhas):
    """Tabela Markdown; `|` e quebras de linha são escapados para não rasgar a tabela."""
    def celula(valor):
        return str(valor if valor is not None else "").replace("|", "\\|").replace("\n", " ")
    saida = ["| " + " | ".join(cabecalho) + " |", "|" + "---|" * len(cabecalho)]
    saida += ["| " + " | ".join(celula(c) for c in linha) + " |" for linha in linhas]
    return saida


def _observou_429(registros_ratemeter):
    """True se algum registro do ratemeter terminou em HTTP 429."""
    return any(r.get("status") == 429 for r in registros_ratemeter)


def _secao_resumo(dados, r):
    """Contadores do topo: é o que o leitor com pressa lê."""
    rotas = dados.get("rotas", [])
    download = dados.get("download", [])
    vazamentos = dados.get("jsleak", [])
    linhas = ["## Resumo executivo", ""]
    if rotas:
        por_kind = {}
        for rota in rotas:
            por_kind[rota.get("kind") or "?"] = por_kind.get(rota.get("kind") or "?", 0) + 1
        detalhe = ", ".join(f"{k}: {por_kind[k]}" for k in sorted(por_kind, key=lambda k: _posicao(k, ORDEM_KINDS)))
        linhas.append(f"- Rotas mapeadas: **{len(rotas)}** ({detalhe})")
    if download:
        sensiveis = sum(1 for d in download if d.get("sensitive"))
        linhas.append(f"- Recursos no manifesto do download-dump: **{len(download)}**")
        linhas.append(f"- Arquivos sensíveis sinalizados: **{sensiveis}**")
    if vazamentos:
        por_sev = {}
        for v in vazamentos:
            por_sev[v.get("severity") or "?"] = por_sev.get(v.get("severity") or "?", 0) + 1
        detalhe = ", ".join(f"{k}: {por_sev[k]}" for k in sorted(por_sev, key=lambda k: _posicao(k, ORDEM_SEVERIDADE)))
        linhas.append(f"- Segredos/vazamentos: **{len(vazamentos)}** ({detalhe})")
    certspy = dados.get("certspy", [])
    if certspy:
        vivos = sum(1 for d in certspy if d.get("vivo"))
        linhas.append(f"- Domínios por SSL/CT (certspy): **{len(certspy)}** (vivos: {vivos})")
    pior = _pior_nota(dados.get("headeraudit", []))
    if pior:
        linhas.append(f"- Pior nota de postura HTTP: **{r(pior)}**")
    if dados.get("ratemeter"):
        linhas.append("- HTTP 429 observado: **" + ("sim" if _observou_429(dados["ratemeter"]) else "não") + "**")
    return linhas if len(linhas) > 2 else []


def _secao_superficie(dados, r):
    rotas = dados.get("rotas", [])
    download = dados.get("download", [])
    if not rotas and not download:
        return []
    linhas = ["## Superfície mapeada", ""]
    if rotas:
        por_kind = {}
        for rota in rotas:
            por_kind[rota.get("kind") or "?"] = por_kind.get(rota.get("kind") or "?", 0) + 1
        linhas += _tabela(["Tipo", "Rotas"], [
            (k, por_kind[k]) for k in sorted(por_kind, key=lambda k: _posicao(k, ORDEM_KINDS))])
        linhas.append("")
    if download:
        linhas.append(f"Total de recursos registrados pelo download-dump: **{len(download)}**.")
    return linhas


def _secao_certspy(dados, r):
    doms = dados.get("certspy", [])
    if not doms:
        return []
    vivos = sum(1 for d in doms if d.get("vivo"))
    linhas = ["## Domínios por SSL/CT (certspy)", "",
              "Descobertos a partir de certificados públicos (CT/crt.sh) e dos SANs "
              f"do certificado ao vivo: **{len(doms)}**, dos quais **{vivos}** no ar (A/AAAA).", ""]
    linhas += _tabela(["Domínio", "Vivo", "Fontes", "IPs"], [
        (r(d.get("nome")), "sim" if d.get("vivo") else "não",
         ", ".join(d.get("fontes") or []), r(", ".join(d.get("ips") or [])))
        for d in doms])
    return linhas


def _secao_sensiveis(dados, r):
    sensiveis = [d for d in dados.get("download", []) if d.get("sensitive")]
    if not sensiveis:
        return []
    linhas = ["## Arquivos sensíveis sinalizados", "",
              "Somente URL e nota; o conteúdo nunca foi baixado.", ""]
    linhas += _tabela(["URL", "Nota"], [(r(s.get("url")), r(s.get("sensitive_note"))) for s in sensiveis])
    return linhas


def _secao_segredos(dados, r):
    vazamentos = dados.get("jsleak", [])
    if not vazamentos:
        return []
    linhas = ["## Segredos e vazamentos", ""]
    severidades = sorted({v.get("severity") or "?" for v in vazamentos},
                         key=lambda s: _posicao(s, ORDEM_SEVERIDADE))
    for severidade in severidades:
        grupo = [v for v in vazamentos if (v.get("severity") or "?") == severidade]
        linhas += [f"### {severidade} ({len(grupo)})", ""]
        linhas += _tabela(["Regra", "Origem", "Linha", "Trecho"], [
            (v.get("rule"), r(v.get("source")), v.get("line", ""), "`" + r(v.get("match")) + "`")
            for v in grupo])
        linhas.append("")
    return linhas


def _secao_postura(dados, r):
    registros = dados.get("headeraudit", [])
    if not registros:
        return []
    linhas = ["## Postura HTTP", ""]
    linhas += _tabela(["URL", "Nota", "Issues"], [
        (r(h.get("url")), h.get("grade") or "—",
         r("; ".join(f"{i.get('check')} ({i.get('severity')})" for i in h.get("issues") or [])
           or h.get("error") or "nenhuma"))
        for h in registros])
    return linhas


def _secao_tls(dados, r):
    registros = dados.get("tlsscan", [])
    if not registros:
        return []
    linhas = ["## TLS e certificados", ""]
    linhas += _tabela(["Host", "Versões aceitas", "Nota", "Issues"], [
        (r(t.get("host")), ", ".join(t.get("versoes_aceitas") or []) or "—", t.get("grade") or "—",
         r("; ".join(f"{i.get('check')} ({i.get('severity')})" for i in t.get("issues") or [])
           or t.get("erro") or "nenhuma"))
        for t in registros])
    return linhas


def _secao_ratelimit(dados, r):
    registros = dados.get("ratemeter", [])
    if not registros:
        return []
    por_url = {}
    for registro in registros:
        por_url.setdefault(registro.get("url") or "?", []).append(registro)
    linhas_tabela = []
    for url, grupo in por_url.items():
        primeiro_429 = next((g for g in grupo if g.get("status") == 429), None)
        # Qualquer chave com "limit" (X-RateLimit-*, limite...) vira "limite observado";
        # retry_after é tratado à parte.
        limites = {}
        for g in grupo:
            for chave, valor in g.items():
                if "limit" in chave.lower() and chave != "retry_after" and valor not in (None, ""):
                    limites.setdefault(chave, valor)
        linhas_tabela.append((
            r(url), len(grupo),
            f"seq {primeiro_429.get('seq')}" if primeiro_429 else "não observado",
            (primeiro_429 or {}).get("retry_after", "—") if primeiro_429 else "—",
            r(", ".join(f"{k}={v}" for k, v in limites.items()) or "—")))
    return ["## Rate-limit", ""] + _tabela(
        ["URL", "Requisições", "Primeiro 429", "Retry-After", "Limites observados"], linhas_tabela)


def _secao_apendice(dados, r):
    fontes = dados.get("fontes", [])
    if not fontes:
        return []
    return ["## Apêndice: fontes lidas", ""] + _tabela(
        ["Arquivo", "Tipo", "Registros"], [(r(f["caminho"]), f["tipo"], f["registros"]) for f in fontes])


def montar_markdown(dados):
    """Monta o relatório a partir do dicionário `dados`; omite seções vazias.

    Decide a redação: roda `redigir` sobre todo texto vindo dos artefatos, a
    menos que dados["redigir"] seja False. Nunca confia que o artefato já veio
    redigido da ferramenta de origem.
    """
    r = redigir if dados.get("redigir", True) else (lambda texto: "" if texto is None else str(texto))
    partes = [
        ["# Relatório de evidências do pentest", "",
         f"- **Alvo:** {r(dados.get('alvo', 'alvo autorizado'))}",
         f"- **Gerado em:** {dados.get('gerado_em', '')}",
         "",
         "> Teste de segurança **AUTORIZADO** pelo responsável do serviço. "
         "Este relatório contém evidências sensíveis: " +
         ("segredos foram redigidos." if dados.get("redigir", True) else "**segredos NÃO foram redigidos.**")],
        _secao_resumo(dados, r), _secao_superficie(dados, r), _secao_certspy(dados, r),
        _secao_sensiveis(dados, r),
        _secao_segredos(dados, r), _secao_postura(dados, r), _secao_tls(dados, r),
        _secao_ratelimit(dados, r), _secao_apendice(dados, r),
    ]
    return "\n\n".join("\n".join(p) for p in partes if p) + "\n"


# --- Orquestração ----------------------------------------------------------

def _agregar(dados, tipo, caminho, registros):
    """Encaixa os registros lidos no bucket do tipo e registra a fonte no apêndice."""
    chave = {TIPO_SPYDER: "rotas", TIPO_DOWNLOAD: "download"}.get(tipo, tipo)
    dados.setdefault(chave, []).extend(registros)
    dados["fontes"].append({"caminho": str(caminho), "tipo": tipo, "registros": len(registros)})


def _ler_e_agregar_jsonl(dados, caminho, tipo_forcado, avisar):
    """Lê um .jsonl e o agrega; usa `tipo_forcado` (flag explícita) ou a classificação."""
    try:
        registros = ler_jsonl(caminho)
    except OSError as erro:
        avisar(f"aviso: não consegui ler {caminho}: {erro}")
        return
    tipo = tipo_forcado or (classificar_jsonl(registros[0]) if registros else None)
    if tipo is None:
        avisar(f"aviso: {caminho} ignorado (vazio ou formato não reconhecido)")
        return
    _agregar(dados, tipo, caminho, registros)


def _ler_e_agregar_spyder(dados, caminho, avisar):
    try:
        _agregar(dados, TIPO_SPYDER, caminho, ler_spyder_db(caminho))
    except (sqlite3.Error, OSError) as erro:
        avisar(f"aviso: não consegui ler o banco {caminho}: {erro}")


def construir_parser():
    parser = argparse.ArgumentParser(
        description="Gera um relatório Markdown de evidências a partir dos artefatos das ferramentas do pentest.")
    parser.add_argument("--project-dir", default=".", help="diretório a varrer (padrão: .)")
    parser.add_argument("--manifest", action="append", default=[], metavar="PATH",
                        help="manifesto download-dump explícito (repetível)")
    parser.add_argument("--spyder-db", action="append", default=[], metavar="PATH",
                        help="banco spyder explícito (repetível)")
    parser.add_argument("--findings", action="append", default=[], metavar="PATH",
                        help="jsonl de achados explícito, de qualquer ferramenta (repetível)")
    parser.add_argument("--target", help="rótulo do alvo (padrão: título do README.md do projeto)")
    parser.add_argument("--out", help="saída .md (padrão: evidence-<ts>.md)")
    parser.add_argument("--no-redact", action="store_true", help="mantém segredos crus (padrão: redige)")
    parser.add_argument("--quiet", action="store_true", help="não imprime mensagens de progresso")
    return parser


def main(argv=None):
    """Lê os argumentos, coleta os artefatos, monta e grava o relatório."""
    args = construir_parser().parse_args(argv)

    def avisar(mensagem):
        print(mensagem, file=sys.stderr)

    def informar(mensagem):
        if not args.quiet:
            print(mensagem)

    dados = {"fontes": [], "redigir": not args.no_redact}
    explicitos = bool(args.manifest or args.spyder_db or args.findings)

    for caminho in args.manifest:
        _ler_e_agregar_jsonl(dados, caminho, TIPO_DOWNLOAD, avisar)
    for caminho in args.spyder_db:
        _ler_e_agregar_spyder(dados, caminho, avisar)
    for caminho in args.findings:
        _ler_e_agregar_jsonl(dados, caminho, None, avisar)

    # Descoberta só entra quando nada foi passado: caminhos explícitos significam
    # "use exatamente estes", e misturar com varredura geraria duplicatas.
    if not explicitos:
        descobertos = descobrir_artefatos(args.project_dir)
        for caminho in descobertos["jsonl"]:
            _ler_e_agregar_jsonl(dados, caminho, None, avisar)
        for caminho in descobertos["spyder"]:
            _ler_e_agregar_spyder(dados, caminho, avisar)

    if not dados["fontes"]:
        avisar("erro: nenhum artefato encontrado (procurei por download-dump-*.jsonl, spyder-*.db, "
               "jsleak-*.jsonl, headeraudit-*.jsonl, tlsscan-*.jsonl, ratemeter-*.jsonl, certspy-*.jsonl). "
               "Use --project-dir, --manifest, --spyder-db ou --findings.")
        return 2

    agora = datetime.now()
    dados["alvo"] = args.target or descobrir_alvo(args.project_dir)
    dados["gerado_em"] = agora.strftime("%Y-%m-%d %H:%M:%S")
    saida = Path(args.out or f"evidence-{agora.strftime('%Y%m%d-%H%M%S')}.md")
    saida.write_text(montar_markdown(dados), encoding="utf-8")
    informar(f"relatório gravado em {saida} ({len(dados['fontes'])} fonte(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
