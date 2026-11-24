package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed wordlists/common.txt
var embeddedCommon string

// catálogo público baixado sozinho (SecLists) — listas modestas e de alto sinal.
// O usuário amplia com --wordlist (local) e --wordlist-url (remota).
var catalogo = map[string]string{
	"seclists-common.txt":        "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/common.txt",
	"seclists-api-endpoints.txt": "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/api/api-endpoints.txt",
}

// lerPaths extrai os caminhos de uma wordlist: uma linha cada, sem comentários
// (#) nem linhas vazias, já aparados.
func lerPaths(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// mesclarPaths junta grupos de caminhos removendo duplicatas e preservando a
// ordem da primeira aparição.
func mesclarPaths(grupos ...[]string) []string {
	visto := make(map[string]bool)
	var out []string
	for _, g := range grupos {
		for _, p := range g {
			if !visto[p] {
				visto[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// baixarSeFaltar baixa url pra dest só se o arquivo ainda não existir (cache).
func baixarSeFaltar(client *http.Client, url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil // já em cache
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d ao baixar %s", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 20*1024*1024)); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, dest) // rename atômico: nunca deixa cache meio-escrito
}

// cacheWordlists é onde as listas baixadas ficam entre execuções.
func cacheWordlists() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "spyder", "wordlists")
	}
	return ".spyder-wordlists"
}

// carregarPaths monta a lista final de caminhos a probar: embutida + catálogo
// baixado (cacheado) + --wordlist (locais) + --wordlist-url (remotas), tudo
// deduplicado. Falha de download não derruba o resto — só avisa.
func carregarPaths(cfg Config, stdout io.Writer) []string {
	grupos := [][]string{lerPaths(strings.NewReader(embeddedCommon))}

	client := &http.Client{Timeout: 30 * time.Second}
	cache := cacheWordlists()

	if cfg.Fetch {
		for nome, url := range catalogo {
			dest := filepath.Join(cache, nome)
			if err := baixarSeFaltar(client, url, dest); err != nil {
				fmt.Fprintf(stdout, "wordlist: falhou baixar %s (%v) — seguindo sem ela\n", nome, err)
				continue
			}
			grupos = append(grupos, lerArquivo(dest))
		}
	}
	for _, u := range cfg.WordlistURLs {
		dest := filepath.Join(cache, "url-"+sanitizarNome(u))
		if err := baixarSeFaltar(client, u, dest); err != nil {
			fmt.Fprintf(stdout, "wordlist: falhou baixar %s (%v)\n", u, err)
			continue
		}
		grupos = append(grupos, lerArquivo(dest))
	}
	for _, p := range cfg.Wordlists {
		grupos = append(grupos, lerArquivo(p))
	}
	return mesclarPaths(grupos...)
}

func lerArquivo(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return lerPaths(f)
}

func sanitizarNome(s string) string {
	r := strings.NewReplacer("/", "_", ":", "_", "?", "_", "&", "_", "=", "_")
	return r.Replace(s)
}
