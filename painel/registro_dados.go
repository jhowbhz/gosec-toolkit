package main

// registroPadrao descreve as 12 ferramentas do kit. Os campos expõem as flags
// principais de cada uma; como o processo roda com o diretório de trabalho na
// pasta do run, os arquivos de saída (jsonl, txt, md, espelho, db) caem lá
// sozinhos e viram artefatos. Campos run-ref apontam para artefatos de runs
// anteriores (compõem as ferramentas), validados contra a raiz dos runs.
func registroPadrao() []ferramenta {
	return []ferramenta{
		{
			ID: "download-dump", Nome: "download-dump", Lang: "go",
			Descricao: "Espelha um site para navegação offline; respeita robots.txt; sinaliza arquivos sensíveis.",
			Campos: []campo{
				{Flag: "url", Label: "URL semente", Tipo: "texto", Required: true, Ajuda: "com scheme, ex.: https://site.com/"},
				{Flag: "scope", Label: "Escopo", Tipo: "enum", Opcoes: []string{"host", "domain", "prefix"}, Default: "host"},
				{Flag: "depth", Label: "Profundidade", Tipo: "int", Default: "-1", Ajuda: "-1 = ilimitado"},
				{Flag: "concurrency", Label: "Concorrência", Tipo: "int", Default: "4"},
				{Flag: "delay", Label: "Delay (ms)", Tipo: "int", Default: "0"},
				{Flag: "max-body", Label: "Máx. corpo (KB)", Tipo: "int", Default: "0", Ajuda: "0 = sem limite"},
				{Flag: "include", Label: "Incluir (regex)", Tipo: "texto"},
				{Flag: "exclude", Label: "Excluir (regex)", Tipo: "texto"},
				{Flag: "header", Label: "Header", Tipo: "texto", Repetivel: true, Ajuda: "Nome: Valor"},
				{Flag: "user-agent", Label: "User-Agent", Tipo: "texto"},
				{Flag: "probe-sensitive", Label: "Sondar sensíveis", Tipo: "bool"},
				{Flag: "ignore-robots", Label: "Ignorar robots.txt", Tipo: "bool"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "spyder-credentials", Nome: "spyder-credentials", Lang: "go", Posicional: true,
			Descricao: "Varre conteúdo e reporta credenciais expostas (classes secreto/publicável/suspeito).",
			Campos: []campo{
				{Flag: "caminho", Label: "Pasta/arquivo a varrer", Tipo: "run-ref", RunTipo: "dir", Posicional: true, Repetivel: true, Ajuda: "ex.: o espelho de um run do download-dump"},
				{Flag: "url", Label: "URL avulsa", Tipo: "texto", Repetivel: true},
				{Flag: "header", Label: "Header", Tipo: "texto", Repetivel: true},
				{Flag: "entropy", Label: "Entropia (ruidoso)", Tipo: "bool"},
				{Flag: "only-secrets", Label: "Só secretos", Tipo: "bool"},
				{Flag: "show-full", Label: "Mostrar valor inteiro", Tipo: "bool"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "tlsscan", Nome: "tlsscan", Lang: "go",
			Descricao: "Inspeciona TLS/cert: versões, ciphers, cadeia, validade, HSTS; nota A–F.",
			Campos: []campo{
				{Flag: "host", Label: "Host", Tipo: "texto", Repetivel: true, Required: true, Ajuda: "HOST[:PORT]"},
				{Flag: "port", Label: "Porta padrão", Tipo: "int", Default: "443"},
				{Flag: "servername", Label: "SNI", Tipo: "texto"},
				{Flag: "timeout", Label: "Timeout (s)", Tipo: "int", Default: "15"},
				{Flag: "full-chain", Label: "Cadeia completa", Tipo: "bool"},
				{Flag: "no-hsts", Label: "Sem checar HSTS", Tipo: "bool"},
			},
		},
		{
			ID: "headeraudit", Nome: "headeraudit", Lang: "go",
			Descricao: "Audita postura HTTP: headers de segurança, cookies e reflexão de CORS; nota A–F.",
			Campos: []campo{
				{Flag: "url", Label: "URL", Tipo: "texto", Repetivel: true, Ajuda: "com scheme"},
				{Flag: "jsonl", Label: "Manifesto (run-ref)", Tipo: "run-ref", RunTipo: "arquivo", Ajuda: "jsonl do download-dump pra extrair URLs"},
				{Flag: "origin", Label: "Origin (CORS)", Tipo: "texto"},
				{Flag: "header", Label: "Header extra", Tipo: "texto", Repetivel: true},
				{Flag: "concurrency", Label: "Concorrência", Tipo: "int"},
				{Flag: "delay", Label: "Delay (ms)", Tipo: "int"},
				{Flag: "timeout", Label: "Timeout (s)", Tipo: "int"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "jsleak", Nome: "jsleak", Lang: "go", Posicional: false,
			Descricao: "Varre arquivos/URLs atrás de segredos e endpoints vazados; mostra redigido.",
			Campos: []campo{
				{Flag: "dir", Label: "Diretório (run-ref)", Tipo: "run-ref", RunTipo: "dir", Repetivel: true},
				{Flag: "file", Label: "Arquivo (run-ref)", Tipo: "run-ref", RunTipo: "arquivo", Repetivel: true},
				{Flag: "url", Label: "URL", Tipo: "texto", Repetivel: true},
				{Flag: "header", Label: "Header", Tipo: "texto", Repetivel: true},
				{Flag: "ext", Label: "Extensões", Tipo: "texto", Ajuda: "csv, ex.: js,json,html"},
				{Flag: "max-body", Label: "Máx. corpo (KB)", Tipo: "int"},
				{Flag: "context", Label: "Contexto (chars)", Tipo: "int"},
				{Flag: "no-redact", Label: "Não redigir", Tipo: "bool"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "jwtinspect", Nome: "jwtinspect", Lang: "go",
			Descricao: "Decodifica e audita JWTs (alg, claims, validade); testa HMAC fraco offline.",
			Campos: []campo{
				{Flag: "token", Label: "Token JWT", Tipo: "texto", Repetivel: true},
				{Flag: "token-file", Label: "Arquivo de tokens (run-ref)", Tipo: "run-ref", RunTipo: "arquivo"},
				{Flag: "secret", Label: "Segredo candidato", Tipo: "texto", Repetivel: true},
				{Flag: "secrets", Label: "Wordlist (run-ref)", Tipo: "run-ref", RunTipo: "arquivo"},
				{Flag: "leeway", Label: "Folga de relógio (s)", Tipo: "int"},
				{Flag: "max-life", Label: "Vida máx. (h)", Tipo: "int"},
			},
		},
		{
			ID: "ratemeter", Nome: "ratemeter", Lang: "go",
			Descricao: "Mede rate-limit/quota com rampa conservadora; para no primeiro 429. Não é DoS.",
			Campos: []campo{
				{Flag: "url", Label: "Endpoint", Tipo: "texto", Required: true, Ajuda: "com scheme"},
				{Flag: "method", Label: "Método", Tipo: "enum", Opcoes: []string{"GET", "POST", "PUT", "DELETE", "HEAD", "PATCH"}, Default: "GET"},
				{Flag: "rate", Label: "Taxa (req/s)", Tipo: "int"},
				{Flag: "total", Label: "Total de requests", Tipo: "int"},
				{Flag: "concurrency", Label: "Concorrência", Tipo: "int"},
				{Flag: "ramp", Label: "Rampa", Tipo: "bool"},
				{Flag: "ramp-steps", Label: "Degraus da rampa", Tipo: "int"},
				{Flag: "duration", Label: "Duração (s)", Tipo: "int"},
				{Flag: "body", Label: "Corpo", Tipo: "texto"},
				{Flag: "timeout", Label: "Timeout (s)", Tipo: "int"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "certspy", Nome: "certspy", Lang: "go",
			Descricao: "Descobre domínios/subdomínios por Certificate Transparency (crt.sh) + SANs ao vivo.",
			Campos: []campo{
				{Flag: "domain", Label: "Domínio", Tipo: "texto", Repetivel: true, Ajuda: "alvo no CT"},
				{Flag: "host", Label: "Host ao vivo", Tipo: "texto", Repetivel: true, Ajuda: "HOST[:PORT] para SANs"},
				{Flag: "no-ct", Label: "Sem crt.sh", Tipo: "bool"},
				{Flag: "no-live", Label: "Sem handshake", Tipo: "bool"},
				{Flag: "no-resolve", Label: "Sem DNS", Tipo: "bool"},
				{Flag: "port", Label: "Porta", Tipo: "int", Default: "443"},
				{Flag: "concurrency", Label: "Concorrência", Tipo: "int"},
				{Flag: "timeout", Label: "Timeout (s)", Tipo: "int"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
			},
		},
		{
			ID: "apidiff", Nome: "apidiff", Lang: "go",
			Descricao: "Compara dois scans (jsonl do dump ou db do spyder) e mostra o drift da superfície.",
			Campos: []campo{
				{Flag: "old", Label: "Scan antigo (run-ref)", Tipo: "run-ref", RunTipo: "arquivo", Required: true},
				{Flag: "new", Label: "Scan novo (run-ref)", Tipo: "run-ref", RunTipo: "arquivo", Required: true},
				{Flag: "format", Label: "Formato", Tipo: "enum", Opcoes: []string{"text", "jsonl"}, Default: "text"},
				{Flag: "only", Label: "Só tipos", Tipo: "texto", Ajuda: "csv: added,removed,changed"},
			},
		},
		{
			ID: "proxyreverse", Nome: "proxyreverse", Lang: "go",
			Descricao: "Proxy reverso de interceptação: grava cada req/resp num log. Fica no ar até parar.",
			Campos: []campo{
				{Flag: "from", Label: "From (host que escuta)", Tipo: "texto", Required: true},
				{Flag: "to", Label: "To (destino, com scheme)", Tipo: "texto", Required: true},
				{Flag: "http", Label: "Escuta HTTP", Tipo: "texto", Ajuda: "ex.: 127.0.0.1:8080"},
				{Flag: "https", Label: "Escuta HTTPS", Tipo: "texto", Ajuda: "ex.: 127.0.0.1:8443"},
				{Flag: "upstream-addr", Label: "Upstream IP:porta", Tipo: "texto"},
				{Flag: "preserve-host", Label: "Preservar Host", Tipo: "bool"},
				{Flag: "insecure", Label: "TLS inseguro", Tipo: "bool"},
				{Flag: "max-body", Label: "Máx. corpo logado (KB)", Tipo: "int", Default: "256"},
			},
		},
		{
			ID: "spyder", Nome: "spyder", Lang: "go",
			Descricao: "Mapeia rotas/endpoints de API via Chrome headless. Precisa de Chrome; pode ser longo.",
			Campos: []campo{
				{Flag: "url", Label: "Seed", Tipo: "texto", Required: true, Ajuda: "com scheme"},
				{Flag: "scope", Label: "Escopo", Tipo: "enum", Opcoes: []string{"same-host", "subdomains"}, Default: "same-host"},
				{Flag: "max-pages", Label: "Máx. páginas", Tipo: "int", Default: "300"},
				{Flag: "max-depth", Label: "Profundidade", Tipo: "int", Default: "4"},
				{Flag: "concurrency", Label: "Abas paralelas", Tipo: "int", Default: "2"},
				{Flag: "delay", Label: "Delay", Tipo: "texto", Default: "300ms", Ajuda: "ex.: 300ms, 1s"},
				{Flag: "api-only", Label: "Só API", Tipo: "bool", Default: "true"},
				{Flag: "ignore-robots", Label: "Ignorar robots.txt", Tipo: "bool"},
				{Flag: "chrome", Label: "Caminho do Chrome", Tipo: "texto"},
			},
		},
		{
			ID: "evidence", Nome: "evidence", Lang: "python", Script: "evidence.py",
			Descricao: "Capstone: lê os artefatos das outras ferramentas e monta um relatório Markdown.",
			Campos: []campo{
				{Flag: "project-dir", Label: "Pasta a varrer (run-ref)", Tipo: "run-ref", RunTipo: "dir", Ajuda: "aponte para um run anterior"},
				{Flag: "target", Label: "Rótulo do alvo", Tipo: "texto"},
				{Flag: "no-redact", Label: "Não redigir", Tipo: "bool"},
			},
		},
	}
}
