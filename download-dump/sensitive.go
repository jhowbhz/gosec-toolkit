package main

import (
	"net/url"
	"path"
	"strings"
)

// sensitive classifica URLs que, num alvo real, não deveriam estar expostas:
// arquivos de ambiente, repositório de versão, chaves, dumps, backups, logs.
// A política do download-dump é NÃO baixar o conteúdo desses — só registrar
// que existem — para evitar exfiltrar segredo durante um teste autorizado.

// nomesSensiveis casam com o último segmento do caminho (minúsculo), exato.
var nomesSensiveis = map[string]string{
	".htpasswd":          "credenciais HTTP (.htpasswd)",
	".htaccess":          "config Apache (.htaccess)",
	".netrc":             "credenciais de rede (.netrc)",
	".npmrc":             "token npm (.npmrc)",
	"id_rsa":             "chave privada SSH",
	"id_dsa":             "chave privada SSH",
	"id_ecdsa":           "chave privada SSH",
	".ds_store":          "metadados do macOS (.DS_Store)",
	"wp-config.php":      "config do WordPress (segredos de banco)",
	"web.config":         "config do IIS/.NET",
	"settings.py":        "config Django (possível SECRET_KEY)",
	"appsettings.json":   "config .NET (possíveis segredos)",
	"docker-compose.yml": "compose (possíveis segredos de ambiente)",
	"credentials":        "arquivo de credenciais",
}

// extensoesSensiveis casam com a extensão do último segmento.
var extensoesSensiveis = map[string]string{
	".pem":    "chave/certificado",
	".key":    "chave privada",
	".p12":    "keystore PKCS#12",
	".pfx":    "keystore PKCS#12",
	".ppk":    "chave PuTTY",
	".sql":    "dump de banco",
	".bak":    "backup",
	".old":    "backup (.old)",
	".backup": "backup",
	".orig":   "backup (.orig)",
	".swp":    "swap de editor (vim)",
	".swo":    "swap de editor (vim)",
	".log":    "arquivo de log",
	".zip":    "arquivo compactado (possível backup/código)",
	".tar":    "arquivo compactado (possível backup/código)",
	".gz":     "arquivo compactado (possível backup/código)",
	".tgz":    "arquivo compactado (possível backup/código)",
	".rar":    "arquivo compactado (possível backup/código)",
	".7z":     "arquivo compactado (possível backup/código)",
	".dump":   "dump",
}

// fragmentosSensiveis casam se o caminho (minúsculo) os contém.
var fragmentosSensiveis = map[string]string{
	"/.git":     "repositório Git exposto",
	"/.svn":     "repositório SVN exposto",
	"/.hg":      "repositório Mercurial exposto",
	"/.idea/":   "config do IntelliJ (.idea)",
	"/.vscode/": "config do VS Code (.vscode)",
	"/.aws":     "credenciais AWS (.aws)",
}

// ehSensivel diz se a URL aponta para um recurso sensível e por quê. A ordem
// — nome exato, fragmento no caminho, extensão, prefixo .env — vai do sinal
// mais específico ao mais genérico.
func ehSensivel(u *url.URL) (bool, string) {
	lp := strings.ToLower(u.Path)
	seg := strings.ToLower(path.Base(u.Path))

	if seg == ".env" || strings.HasPrefix(seg, ".env.") {
		return true, "arquivo de ambiente (.env)"
	}
	if motivo, ok := nomesSensiveis[seg]; ok {
		return true, motivo
	}
	for frag, motivo := range fragmentosSensiveis {
		if strings.Contains(lp, frag) {
			return true, motivo
		}
	}
	if motivo, ok := extensoesSensiveis[path.Ext(seg)]; ok {
		return true, motivo
	}
	return false, ""
}

// caminhosSensiveisComuns é a wordlist da sonda ativa opt-in (--probe-sensitive):
// caminhos clássicos cuja simples existência num servidor já é um achado. A
// existência é confirmada contra a linha de base de soft-404 (ver crawler.go);
// o conteúdo nunca é baixado. Lista curada de propósito — não é um dirbuster.
func caminhosSensiveisComuns() []string {
	return []string{
		// variáveis de ambiente
		".env",
		".env.local",
		".env.dev",
		".env.development",
		".env.prod",
		".env.production",
		".env.staging",
		".env.test",
		".env.backup",
		".env.bak",
		".env.save",
		".env.old",
		".env.example",

		// controle de versão exposto
		".git/config",
		".git/HEAD",
		".git/logs/HEAD",
		".git/index",
		".gitignore",
		".svn/entries",
		".svn/wc.db",
		".hg/store/00manifest.i",
		".bzr/branch/branch.conf",

		// chaves, certificados e credenciais
		"id_rsa",
		"id_dsa",
		"id_ecdsa",
		"id_ed25519",
		".ssh/id_rsa",
		".ssh/authorized_keys",
		".ssh/known_hosts",
		".htpasswd",
		".htaccess",
		".netrc",
		".npmrc",
		".pgpass",
		"server.key",
		"private.key",
		"privatekey.pem",
		"cert.pem",
		".aws/credentials",
		".aws/config",
		"credentials.json",
		"secrets.json",
		"secrets.yml",
		"secrets.yaml",

		// configs de framework com segredos
		"wp-config.php",
		"wp-config.php.bak",
		"wp-config.php.old",
		"wp-config.php.save",
		"wp-config.php~",
		"wp-config.php.swp",
		"configuration.php",
		"config.php",
		"config.php.bak",
		"config.inc.php",
		"config/database.yml",
		"config/secrets.yml",
		"config/master.key",
		"app/etc/env.php",
		"settings.py",
		"local_settings.py",
		"appsettings.json",
		"appsettings.Development.json",
		"web.config",
		"application.properties",
		"application.yml",
		"docker-compose.yml",
		"docker-compose.override.yml",
		"Dockerfile",
		".dockercfg",
		".docker/config.json",

		// dumps e bancos
		"backup.sql",
		"database.sql",
		"dump.sql",
		"db.sql",
		"mysql.sql",
		"data.sql",
		"backup.sql.gz",
		"database.sqlite",
		"db.sqlite3",

		// backups e arquivos compactados
		"backup.zip",
		"backup.tar.gz",
		"backup.tar",
		"backup.rar",
		"backup.7z",
		"www.zip",
		"site.zip",
		"public_html.zip",
		"web.zip",
		"release.zip",
		"archive.zip",

		// logs e info disclosure
		"error.log",
		"debug.log",
		"access.log",
		"laravel.log",
		"storage/logs/laravel.log",
		"npm-debug.log",
		"phpinfo.php",
		"info.php",
		"test.php",
		"adminer.php",
		"phpmyadmin/",
		"server-status",
		"composer.json",
		"composer.lock",
		"package.json",
		"yarn.lock",
	}
}
