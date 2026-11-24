package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := envOr("PAINEL_ADDR", "127.0.0.1:7777")
	token := os.Getenv("PAINEL_TOKEN")
	toolsDir := envOr("PAINEL_TOOLS_DIR", "..")
	for i, a := range os.Args[1:] {
		_ = i
		if v, ok := corta(a, "--addr="); ok {
			addr = v
		} else if v, ok := corta(a, "--token="); ok {
			token = v
		} else if v, ok := corta(a, "--tools-dir="); ok {
			toolsDir = v
		}
	}
	if token == "" {
		token = gerarToken()
	}
	td, _ := filepath.Abs(toolsDir)

	st := novoStore("runs")
	if err := os.MkdirAll("runs", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "painel: não consegui criar runs/:", err)
		os.Exit(1)
	}
	rn := novoRunner(st, func(f ferramenta) (string, []string, error) { return resolverBin(td, f) })
	ui, _ := fs.Sub(webFS, "web")
	h := novoServidor(token, registroPadrao(), rn, st, ui)

	if !ehLoopback(addr) {
		fmt.Fprintln(os.Stderr, "painel: AVISO — escutando fora de loopback; qualquer um na rede pode rodar as ferramentas.")
	}
	url := fmt.Sprintf("http://%s/?token=%s", addr, token)
	fmt.Println("painel no ar. Abra no navegador (o token já vai na URL):")
	fmt.Println("  " + url)
	fmt.Println("ferramentas em:", td)
	fmt.Println("Ctrl+C encerra.")
	if err := http.ListenAndServe(addr, h); err != nil {
		fmt.Fprintln(os.Stderr, "painel:", err)
		os.Exit(1)
	}
}

// resolverBin acha (ou compila) o executável de uma ferramenta Go, ou monta a
// invocação do script Python. Devolve o binário e o prefixo de argv.
func resolverBin(toolsDir string, f ferramenta) (string, []string, error) {
	dir := filepath.Join(toolsDir, f.ID)
	if f.Lang == "python" {
		py := acharPython()
		if py == "" {
			return "", nil, fmt.Errorf("python não encontrado no PATH")
		}
		return py, []string{"-I", filepath.Join(dir, f.Script)}, nil
	}
	bin := filepath.Join(dir, f.ID)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); err != nil {
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = dir
		if out, err := build.CombinedOutput(); err != nil {
			return "", nil, fmt.Errorf("falha ao compilar %s: %v\n%s", f.ID, err, out)
		}
	}
	return bin, nil, nil
}

func acharPython() string {
	for _, c := range []string{"python", "python3"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func gerarToken() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func ehLoopback(addr string) bool {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	return host == "" || host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "[::1]"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func corta(s, prefixo string) (string, bool) {
	if strings.HasPrefix(s, prefixo) {
		return s[len(prefixo):], true
	}
	return "", false
}
