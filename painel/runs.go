package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// runs guarda cada execução em disco: runs/<id>/ com meta.json, output.log e
// os artefatos que a ferramenta gerou. O painel aponta as flags de saída da
// ferramenta para essa pasta, então tudo de uma execução fica reunido ali.

// run é o registro de uma execução.
type run struct {
	ID     string              `json:"id"`
	Tool   string              `json:"tool"`
	Args   map[string][]string `json:"args,omitempty"`
	Argv   []string            `json:"argv"`
	Inicio time.Time           `json:"inicio"`
	Fim    *time.Time          `json:"fim,omitempty"`
	Exit   *int                `json:"exit,omitempty"`
	Status string              `json:"status"` // rodando | ok | erro
}

// store persiste e lê os runs sob uma pasta raiz.
type store struct {
	raiz string
	mu   sync.Mutex
}

func novoStore(raiz string) *store {
	return &store{raiz: raiz}
}

func (s *store) dirDoRun(id string) string {
	return filepath.Join(s.raiz, id)
}

// criarRun gera um id, cria a pasta e grava o meta inicial (status rodando).
func (s *store) criarRun(tool string, args map[string][]string, argv []string) (run, error) {
	r := run{
		ID:     novoID(),
		Tool:   tool,
		Args:   args,
		Argv:   argv,
		Inicio: time.Now(),
		Status: "rodando",
	}
	if err := os.MkdirAll(s.dirDoRun(r.ID), 0o755); err != nil {
		return run{}, err
	}
	if err := s.salvarMeta(r); err != nil {
		return run{}, err
	}
	return r, nil
}

// salvarMeta grava (ou regrava) o meta.json do run.
func (s *store) salvarMeta(r run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dirDoRun(r.ID), "meta.json"), b, 0o644)
}

// obter lê o meta.json de um run.
func (s *store) obter(id string) (run, error) {
	b, err := os.ReadFile(filepath.Join(s.dirDoRun(id), "meta.json"))
	if err != nil {
		return run{}, err
	}
	var r run
	if err := json.Unmarshal(b, &r); err != nil {
		return run{}, err
	}
	return r, nil
}

// listar devolve os runs do mais recente ao mais antigo.
func (s *store) listar() []run {
	entradas, err := os.ReadDir(s.raiz)
	if err != nil {
		return nil
	}
	var rs []run
	for _, e := range entradas {
		if !e.IsDir() {
			continue
		}
		if r, err := s.obter(e.Name()); err == nil {
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].Inicio.After(rs[j].Inicio) })
	return rs
}

// artefatos lista os arquivos gerados num run (caminhos relativos, com '/'),
// menos o meta.json.
func (s *store) artefatos(id string) []string {
	base := s.dirDoRun(id)
	var out []string
	filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil || rel == "meta.json" {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

// caminhoArtefato resolve um artefato relativo garantindo que ele fica DENTRO
// da pasta do run — barra path traversal e caminho absoluto.
func (s *store) caminhoArtefato(id, rel string) (string, error) {
	// Rejeita, em qualquer OS: vazio, absoluto, barra/contrabarra inicial
	// (root), drive (":") e travessia (".."). No Windows "/etc" não conta como
	// absoluto, por isso a checagem explícita de prefixo.
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") ||
		strings.ContainsAny(rel[:1], "/\\") || strings.Contains(rel, ":") {
		return "", errors.New("caminho de artefato inválido")
	}
	base, err := filepath.Abs(s.dirDoRun(id))
	if err != nil {
		return "", err
	}
	full := filepath.Join(base, filepath.FromSlash(rel))
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", errors.New("caminho fora do run")
	}
	return full, nil
}

// dentro diz se um caminho fica sob a raiz dos runs — usado para validar os
// campos run-ref (composição entre ferramentas), impedindo apontar a leitura
// de uma ferramenta para um caminho arbitrário do disco.
func (s *store) dentro(p string) bool {
	raiz, err := filepath.Abs(s.raiz)
	if err != nil {
		return false
	}
	ap, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	return ap == raiz || strings.HasPrefix(ap, raiz+string(os.PathSeparator))
}

// novoID devolve um identificador ordenável por tempo e único.
func novoID() string {
	var b [3]byte
	rand.Read(b[:])
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}
