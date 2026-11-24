package main

import (
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// storage decide onde cada URL vira arquivo no espelho e grava os bytes. O
// mapeamento URL -> caminho local precisa ser determinístico: o passe de
// reescrita (rewrite.go) depende dele para apontar um recurso ao outro.

// charsIlegais são os proibidos em nome de arquivo no Windows (e chatos no
// resto); viram '_' para o espelho abrir em qualquer sistema.
const charsIlegais = `<>:"/\|?*`

// localPath traduz uma URL no caminho relativo (com '/') dentro do espelho:
// <host>/<caminho>. Diretório (termina em '/') vira index.html; página sem
// extensão ganha a extensão do content-type; a query entra no nome para não
// colidir dois recursos de mesmo caminho.
func localPath(u *url.URL, contentType string) string {
	host := sanitizarSegmento(strings.ReplaceAll(u.Host, ":", "_"))

	terminaBarra := u.Path == "" || strings.HasSuffix(u.Path, "/")
	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s == "" {
			continue
		}
		segs = append(segs, sanitizarSegmento(s))
	}

	var dirs []string
	var file string
	if terminaBarra || len(segs) == 0 {
		dirs = segs
		file = "index.html"
	} else {
		dirs = segs[:len(segs)-1]
		file = aplicarExtensao(segs[len(segs)-1], contentType)
	}
	file = inserirQuery(file, u.RawQuery)

	partes := append([]string{host}, dirs...)
	partes = append(partes, file)
	return path.Join(partes...)
}

// salvar grava data em <root>/<rel>, criando os diretórios no caminho, e
// devolve o caminho absoluto gravado.
func salvar(root, rel string, data []byte) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	return full, nil
}

// sanitizarSegmento troca chars ilegais e neutraliza '.'/'..' para que a URL
// nunca escape da pasta do espelho (path traversal) nem quebre no Windows.
func sanitizarSegmento(s string) string {
	if s == "." || s == ".." {
		return strings.Repeat("_", len(s))
	}
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(charsIlegais, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	return out
}

// aplicarExtensao dá uma extensão ao arquivo sem uma: .html para páginas,
// ou a extensão canônica do content-type quando conhecida.
func aplicarExtensao(file, contentType string) string {
	if strings.Contains(file, ".") {
		return file
	}
	if strings.Contains(strings.ToLower(contentType), "html") {
		return file + ".html"
	}
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
			return file + exts[0]
		}
	}
	return file
}

// inserirQuery embute um resumo da query no nome (antes da extensão) para
// que /a.js?v=1 e /a.js?v=2 virem arquivos distintos.
func inserirQuery(file, rawQuery string) string {
	if rawQuery == "" {
		return file
	}
	soma := sha256.Sum256([]byte(rawQuery))
	marca := "__q" + hex.EncodeToString(soma[:])[:8]
	ext := path.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	return stem + marca + ext
}
