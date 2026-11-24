package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// carregarURLs junta as três fontes (--url, --urls-file, --jsonl) numa lista
// única e sem repetição, na ordem em que apareceram. É aqui, e não em
// validate, que o disco é lido.
func carregarURLs(cfg Config) ([]string, error) {
	vistas := map[string]bool{}
	var out []string
	adicionar := func(u string) {
		if !vistas[u] {
			vistas[u] = true
			out = append(out, u)
		}
	}

	for _, u := range cfg.URLs {
		adicionar(u)
	}
	if cfg.URLsFile != "" {
		urls, err := lerArquivo(cfg.URLsFile, lerURLsDeLinhas)
		if err != nil {
			return nil, err
		}
		for _, u := range urls {
			adicionar(u)
		}
	}
	if cfg.JSONL != "" {
		urls, err := lerArquivo(cfg.JSONL, lerURLsDoManifesto)
		if err != nil {
			return nil, err
		}
		for _, u := range urls {
			adicionar(u)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nenhuma URL válida encontrada nas fontes informadas")
	}
	return out, nil
}

func lerArquivo(path string, ler func(io.Reader) ([]string, error)) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("não consegui abrir %s: %w", path, err)
	}
	defer f.Close()
	urls, err := ler(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return urls, nil
}

// lerURLsDeLinhas lê uma URL por linha. Linha vazia e comentário (#) são
// ignorados; linha com URL malformada aborta, porque auditar "metade da
// lista" em silêncio daria a falsa impressão de cobertura completa.
func lerURLsDeLinhas(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		linha := strings.TrimSpace(sc.Text())
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		if err := validarURLAlvo(linha); err != nil {
			return nil, fmt.Errorf("linha %d: %w", n, err)
		}
		out = append(out, linha)
	}
	return out, sc.Err()
}

// lerURLsDoManifesto extrai o campo "url" de cada linha do JSONL do
// download-dump. Ali convivem páginas, assets e links sensíveis; ficam só
// as que são http(s) e que respondem ao GET de uma página de verdade
// (registros sem status ou com erro não têm o que auditar).
func lerURLsDoManifesto(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // linhas do manifesto podem ser longas
	for n := 1; sc.Scan(); n++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var reg struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &reg); err != nil {
			return nil, fmt.Errorf("linha %d: JSON inválido: %v", n, err)
		}
		if reg.Error != "" || reg.Status == 0 || validarURLAlvo(reg.URL) != nil {
			continue
		}
		out = append(out, reg.URL)
	}
	return out, sc.Err()
}
