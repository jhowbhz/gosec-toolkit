package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// sources reúne o que vai ser varrido: arquivos de uma pasta (ex.: o espelho
// do download-dump), URLs avulsas e stdin. Arquivos binários são ignorados —
// segredo mora em texto (JS, HTML, JSON, .env, YAML, source maps).

// fonte é um blob de texto a varrer, com o nome de onde veio (arquivo ou URL).
type fonte struct {
	nome  string
	dados []byte
}

const (
	maxArquivo = 16 << 20 // teto por arquivo/URL: 16 MB
	sniffBytes = 8 << 10  // quanto olhar para decidir texto vs binário
)

// ehProvavelTexto decide se um conteúdo é texto: a presença de um byte NUL no
// começo é um sinal forte de binário. Simples e eficaz para o que varremos.
func ehProvavelTexto(data []byte) bool {
	n := len(data)
	if n > sniffBytes {
		n = sniffBytes
	}
	return !bytes.ContainsRune(data[:n], 0)
}

// lerArquivosDe percorre cada caminho (arquivo ou diretório, recursivo) e
// devolve as fontes de texto encontradas, pulando binários e arquivos acima
// do teto.
func lerArquivosDe(paths []string) ([]fonte, error) {
	var out []fonte
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("não consegui acessar %s: %w", p, err)
		}
		if !info.IsDir() {
			if f, ok := lerArquivo(p, info.Size()); ok {
				out = append(out, f)
			}
			continue
		}
		err = filepath.WalkDir(p, func(caminho string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			if f, ok := lerArquivo(caminho, fi.Size()); ok {
				out = append(out, f)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// lerArquivo lê um arquivo se couber no teto e parecer texto.
func lerArquivo(caminho string, tam int64) (fonte, bool) {
	if tam > maxArquivo {
		return fonte{}, false
	}
	data, err := os.ReadFile(caminho)
	if err != nil || !ehProvavelTexto(data) {
		return fonte{}, false
	}
	return fonte{nome: caminho, dados: data}, true
}

// fetchURL busca uma URL e devolve seu corpo como fonte, aplicando os headers
// configurados (ex.: Authorization para área autenticada) e o User-Agent.
func fetchURL(client *http.Client, alvo string, headers http.Header, ua string) (fonte, error) {
	if _, err := url.ParseRequestURI(alvo); err != nil {
		return fonte{}, fmt.Errorf("URL inválida %q: %w", alvo, err)
	}
	req, err := http.NewRequest(http.MethodGet, alvo, nil)
	if err != nil {
		return fonte{}, err
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fonte{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArquivo))
	if err != nil {
		return fonte{}, err
	}
	return fonte{nome: alvo, dados: data}, nil
}

// lerStdin lê todo o conteúdo de um reader como uma fonte chamada "(stdin)".
func lerStdin(r io.Reader) (fonte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxArquivo))
	if err != nil {
		return fonte{}, err
	}
	return fonte{nome: "(stdin)", dados: data}, nil
}
