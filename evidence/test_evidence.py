"""Testes do evidence: guardam a redação de segredos e a omissão de seções vazias.

Se a redação regredir, o relatório de um teste autorizado vaza segredo real
para quem o ler; por isso ela é o foco principal.
"""

import sqlite3
import tempfile
import unittest
from pathlib import Path

import evidence


class RedigirTest(unittest.TestCase):
    def test_mascara_chave_aws_preservando_prefixo(self):
        saida = evidence.redigir("chave AKIAIOSFODNN7EXAMPLE no bundle")
        self.assertIn("AKIA", saida)
        self.assertNotIn("IOSFODNN7EXAMPLE", saida)
        self.assertIn("…[20 chars]", saida)

    def test_mascara_jwt_google_e_github(self):
        jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.assinaturaSecreta123"
        google = "AIzaSyA1234567890abcdefghijklmnopqrstu"
        github = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
        saida = evidence.redigir(f"{jwt} {google} {github}")
        self.assertNotIn("assinaturaSecreta123", saida)
        self.assertNotIn("1234567890abcdefghijk", saida)
        self.assertNotIn("abcdefghijklmnopqrstuvwxyz", saida)

    def test_mascara_valor_mas_mantem_a_chave(self):
        saida = evidence.redigir('api_key="supersecretvalue99"')
        self.assertIn("api_key", saida)
        self.assertNotIn("supersecretvalue99", saida)

    def test_nao_quebra_texto_sem_segredo(self):
        texto = "https://site.com/painel/usuarios?pagina=2 sem nada de especial"
        self.assertEqual(evidence.redigir(texto), texto)

    def test_aceita_vazio_e_none(self):
        self.assertEqual(evidence.redigir(""), "")
        self.assertIsNone(evidence.redigir(None))


class ClassificarJsonlTest(unittest.TestCase):
    def test_cada_tipo_pela_chave(self):
        casos = {
            evidence.TIPO_DOWNLOAD: {"url": "u", "sha256": "x", "saved_path": "p"},
            evidence.TIPO_JSLEAK: {"rule": "aws", "severity": "high", "match": "m"},
            evidence.TIPO_HEADERAUDIT: {"url": "u", "grade": "B", "issues": []},
            evidence.TIPO_TLSSCAN: {"host": "h", "versoes_aceitas": ["TLS1.2"], "grade": "A"},
            evidence.TIPO_RATEMETER: {"seq": 1, "retry_after": 30, "status": 429},
            evidence.TIPO_CERTSPY: {"nome": "api.alvo.com", "fontes": ["ct"], "vivo": True, "sob_apex": True},
        }
        for esperado, linha in casos.items():
            with self.subTest(tipo=esperado):
                self.assertEqual(evidence.classificar_jsonl(linha), esperado)

    def test_desconhecido_devolve_none(self):
        self.assertIsNone(evidence.classificar_jsonl({"foo": 1}))
        self.assertIsNone(evidence.classificar_jsonl([]))


class LerSpyderDbTest(unittest.TestCase):
    def test_le_as_duas_linhas_da_tabela_routes(self):
        with tempfile.TemporaryDirectory() as tmp:
            caminho = Path(tmp) / "spyder-teste.db"
            conexao = sqlite3.connect(caminho)
            conexao.execute(
                "CREATE TABLE routes (url TEXT NOT NULL, host TEXT, method TEXT NOT NULL DEFAULT 'GET', "
                "status INTEGER, kind TEXT, source_url TEXT, depth INTEGER, title TEXT, first_seen TEXT, "
                "PRIMARY KEY (url, method))")
            conexao.executemany(
                "INSERT INTO routes (url, host, method, status, kind, depth) VALUES (?,?,?,?,?,?)",
                [("https://a.com/", "a.com", "GET", 200, "page", 0),
                 ("https://a.com/api", "a.com", "POST", 401, "xhr", 1)])
            conexao.commit()
            conexao.close()

            rotas = evidence.ler_spyder_db(caminho)

        self.assertEqual(len(rotas), 2)
        self.assertEqual(rotas[0]["url"], "https://a.com/")
        self.assertEqual(rotas[1]["kind"], "xhr")


class MontarMarkdownTest(unittest.TestCase):
    def dados_minimos(self):
        return {
            "alvo": "alvo teste", "gerado_em": "2026-01-01 10:00:00", "redigir": True,
            "rotas": [{"url": "https://a.com/", "kind": "page"}, {"url": "https://a.com/x", "kind": "xhr"}],
            "download": [{"url": "https://a.com/.env", "sensitive": True, "sensitive_note": "env exposto"}],
            "jsleak": [{"rule": "aws", "severity": "high", "source": "https://a.com/app.js",
                        "match": "AKIAIOSFODNN7EXAMPLE", "line": 3}],
            "fontes": [{"caminho": "x.jsonl", "tipo": "jsleak", "registros": 1}],
        }

    def test_produz_secoes_esperadas_e_redige(self):
        md = evidence.montar_markdown(self.dados_minimos())
        for titulo in ("# Relatório de evidências", "## Resumo executivo", "## Superfície mapeada",
                       "## Arquivos sensíveis sinalizados", "## Segredos e vazamentos", "## Apêndice"):
            self.assertIn(titulo, md)
        self.assertNotIn("IOSFODNN7EXAMPLE", md)

    def test_omite_secoes_sem_dados(self):
        md = evidence.montar_markdown(self.dados_minimos())
        for titulo in ("## Postura HTTP", "## TLS e certificados", "## Rate-limit",
                       "## Domínios por SSL/CT (certspy)"):
            self.assertNotIn(titulo, md)

    def test_secao_certspy_quando_ha_dominios(self):
        dados = self.dados_minimos()
        dados["certspy"] = [
            {"nome": "api.alvo.com", "fontes": ["ct", "live"], "vivo": True, "ips": ["203.0.113.5"]},
            {"nome": "old.alvo.com", "fontes": ["ct"], "vivo": False, "ips": []}]
        md = evidence.montar_markdown(dados)
        self.assertIn("## Domínios por SSL/CT (certspy)", md)
        self.assertIn("api.alvo.com", md)
        self.assertIn("Domínios por SSL/CT (certspy): **2** (vivos: 1)", md)

    def test_no_redact_mantem_o_segredo_cru(self):
        dados = self.dados_minimos()
        dados["redigir"] = False
        self.assertIn("AKIAIOSFODNN7EXAMPLE", evidence.montar_markdown(dados))

    def test_resumo_aponta_pior_nota_e_429(self):
        dados = self.dados_minimos()
        dados["headeraudit"] = [{"url": "u1", "grade": "A", "issues": []},
                                {"url": "u2", "grade": "D", "issues": [{"check": "csp", "severity": "high"}]}]
        dados["ratemeter"] = [{"url": "u1", "seq": 5, "status": 429, "retry_after": 30}]
        md = evidence.montar_markdown(dados)
        self.assertIn("Pior nota de postura HTTP: **D**", md)
        self.assertIn("HTTP 429 observado: **sim**", md)
        self.assertIn("## Rate-limit", md)


if __name__ == "__main__":
    unittest.main()
