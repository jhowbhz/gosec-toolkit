package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// entrada é a forma comum dos dois formatos de scan. Method e Kind só existem
// no .db do spyder; Hash só existe no .jsonl do download-dump.
type entrada struct {
	Key    string
	URL    string
	Method string
	Status int
	Kind   string
	Hash   string
}

const (
	tipoJSONL = "jsonl"
	tipoDB    = "db"

	// linhas do manifesto podem ter erro/nota longos; o buffer padrão do
	// Scanner (64 KiB) estouraria com "token too long".
	tetoLinhaJSONL = 4 * 1024 * 1024
)

// tipoDoScan decide o formato pela extensão. É pura de propósito: validate a
// usa sem tocar o disco.
func tipoDoScan(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl":
		return tipoJSONL, nil
	case ".db":
		return tipoDB, nil
	}
	return "", fmt.Errorf("extensão não suportada em %q (quero .jsonl do download-dump ou .db do spyder)", path)
}

// carregar abre o scan e o normaliza num mapa chave -> entrada.
func carregar(path string) (map[string]entrada, error) {
	tipo, err := tipoDoScan(path)
	if err != nil {
		return nil, err
	}
	if tipo == tipoDB {
		return carregarDB(path)
	}
	return carregarJSONL(path)
}

// linhaManifesto é o subconjunto do registro do download-dump que importa
// para o diff. Duplicado de propósito: as ferramentas são módulos separados.
type linhaManifesto struct {
	URL       string `json:"url"`
	Status    int    `json:"status"`
	SHA256    string `json:"sha256"`
	Skipped   bool   `json:"skipped"`
	Sensitive bool   `json:"sensitive"`
	Error     string `json:"error"`
}

// carregarJSONL lê o manifesto do download-dump. Pula pulados, sensíveis e
// erros: não têm conteúdo baixado, então comparar sha256/status deles geraria
// "alteração" falsa. A chave é a URL.
func carregarJSONL(path string) (map[string]entrada, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("não consegui abrir %s: %w", path, err)
	}
	defer f.Close()

	out := map[string]entrada{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), tetoLinhaJSONL)
	for n := 1; sc.Scan(); n++ {
		linha := strings.TrimSpace(sc.Text())
		if linha == "" {
			continue
		}
		var l linhaManifesto
		if err := json.Unmarshal([]byte(linha), &l); err != nil {
			return nil, fmt.Errorf("%s linha %d: JSON inválido: %w", path, n, err)
		}
		if l.URL == "" || l.Skipped || l.Sensitive || l.Error != "" {
			continue
		}
		out[l.URL] = entrada{Key: l.URL, URL: l.URL, Status: l.Status, Hash: l.SHA256}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lendo %s: %w", path, err)
	}
	return out, nil
}

// carregarDB lê a tabela routes do spyder. A chave é url+"\t"+method porque a
// mesma URL com GET e POST são rotas distintas.
func carregarDB(path string) (map[string]entrada, error) {
	// sql.Open com driver sqlite CRIA o arquivo se ele não existir; sem esta
	// checagem um --old com typo viraria um banco vazio e um diff enganoso.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("não consegui abrir %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT url, method, status, kind FROM routes`)
	if err != nil {
		return nil, fmt.Errorf("lendo routes de %s: %w", path, err)
	}
	defer rows.Close()

	out := map[string]entrada{}
	for rows.Next() {
		var url, method string
		var status sql.NullInt64
		var kind sql.NullString
		if err := rows.Scan(&url, &method, &status, &kind); err != nil {
			return nil, fmt.Errorf("lendo linha de %s: %w", path, err)
		}
		chave := url + "\t" + method
		out[chave] = entrada{Key: chave, URL: url, Method: method, Status: int(status.Int64), Kind: kind.String}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lendo %s: %w", path, err)
	}
	return out, nil
}
