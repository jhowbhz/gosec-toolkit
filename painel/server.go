package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
)

// server expõe o painel: serve a UI (sem segredo nenhum) e a API protegida por
// token. A UI aberta é inofensiva — nada funciona sem o token, que só existe na
// URL que o usuário abriu e no sessionStorage do navegador. A API exige o
// header X-Painel-Token (o SSE, que não manda header, aceita ?token=).

type servidor struct {
	token string
	reg   []ferramenta
	rn    *runner
	st    *store
}

// novoServidor monta o roteador. ui é o sistema de arquivos da interface
// (injetável para teste; em produção é o embed).
func novoServidor(token string, reg []ferramenta, rn *runner, st *store, ui fs.FS) http.Handler {
	s := &servidor{token: token, reg: reg, rn: rn, st: st}
	mux := http.NewServeMux()

	// UI (sem auth: é só o shell, sem segredo).
	mux.Handle("GET /", http.FileServer(http.FS(ui)))

	// API (com auth).
	mux.HandleFunc("GET /api/tools", s.auth(s.tools))
	mux.HandleFunc("POST /api/run", s.auth(s.run))
	mux.HandleFunc("GET /api/runs", s.auth(s.runs))
	mux.HandleFunc("GET /api/runs/{id}", s.auth(s.runDetalhe))
	mux.HandleFunc("GET /api/runs/{id}/stream", s.auth(s.stream))
	mux.HandleFunc("GET /api/runs/{id}/file", s.auth(s.file))
	mux.HandleFunc("POST /api/runs/{id}/stop", s.auth(s.stop))
	return mux
}

// validarRunRefs garante que todo valor de campo run-ref aponta para dentro da
// pasta de runs — a composição entre ferramentas nunca lê caminho arbitrário.
func (s *servidor) validarRunRefs(f ferramenta, args map[string][]string) error {
	refs := map[string]bool{}
	for _, c := range f.Campos {
		if c.Tipo == "run-ref" {
			refs[c.Flag] = true
		}
	}
	for flag, vals := range args {
		if !refs[flag] {
			continue
		}
		for _, v := range vals {
			if v != "" && !s.st.dentro(v) {
				return fmt.Errorf("campo %q deve apontar para um artefato de run (dentro de runs/)", flag)
			}
		}
	}
	return nil
}

// auth exige o token no header X-Painel-Token ou na query ?token= (para o SSE,
// que não permite header). Comparação em tempo constante.
func (s *servidor) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("X-Painel-Token")
		if t == "" {
			t = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) != 1 {
			http.Error(w, "token inválido", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *servidor) tools(w http.ResponseWriter, r *http.Request) {
	escreverJSON(w, s.reg)
}

func (s *servidor) run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tool string              `json:"tool"`
		Args map[string][]string `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo inválido", http.StatusBadRequest)
		return
	}
	f, ok := s.ferramenta(req.Tool)
	if !ok {
		http.Error(w, "ferramenta desconhecida", http.StatusNotFound)
		return
	}
	if err := s.validarRunRefs(f, req.Args); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	run, err := s.rn.iniciar(f, req.Args)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	escreverJSON(w, map[string]string{"id": run.ID})
}

func (s *servidor) runs(w http.ResponseWriter, r *http.Request) {
	var out []map[string]any
	for _, run := range s.st.listar() {
		abs, _ := filepath.Abs(s.st.dirDoRun(run.ID))
		out = append(out, map[string]any{
			"run":       run,
			"dir":       abs,
			"artefatos": s.st.artefatos(run.ID),
		})
	}
	escreverJSON(w, out)
}

func (s *servidor) runDetalhe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.st.obter(id)
	if err != nil {
		http.Error(w, "run não encontrado", http.StatusNotFound)
		return
	}
	escreverJSON(w, map[string]any{"run": run, "artefatos": s.st.artefatos(id)})
}

// stream envia a saída ao vivo como Server-Sent Events.
func (s *servidor) stream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ch, cancelar, _ := s.rn.inscrever(id)
	if ch == nil {
		// run já terminou e o fluxo não está mais em memória: manda o log salvo.
		http.Error(w, "fluxo indisponível (veja o output.log do run)", http.StatusGone)
		return
	}
	defer cancelar()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	for linha := range ch {
		fmt.Fprintf(w, "data: %s\n\n", linha)
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprint(w, "event: fim\ndata: fim\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// file serve um artefato do run, restrito à pasta do run.
func (s *servidor) file(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rel := r.URL.Query().Get("path")
	caminho, err := s.st.caminhoArtefato(id, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, caminho)
}

func (s *servidor) stop(w http.ResponseWriter, r *http.Request) {
	ok := s.rn.parar(r.PathValue("id"))
	escreverJSON(w, map[string]bool{"parado": ok})
}

func (s *servidor) ferramenta(id string) (ferramenta, bool) {
	for _, f := range s.reg {
		if f.ID == id {
			return f, true
		}
	}
	return ferramenta{}, false
}

func escreverJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
