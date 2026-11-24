package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

// dashboard é a área web ao vivo: serve a página, a lista atual (SQLite) e um
// fluxo SSE com as rotas novas conforme o crawler as descobre.
type dashboard struct {
	store  *store
	broker *broker
}

func newDashboard(st *store, b *broker) *dashboard {
	return &dashboard{store: st, broker: b}
}

func (d *dashboard) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.página)
	mux.HandleFunc("/api/routes", d.rotas)
	mux.HandleFunc("/events", d.eventos)
	return mux
}

// escutar abre o endereço do dashboard. Feito antes do crawl pra que porta
// ocupada vire erro claro no startup, não um erro perdido numa goroutine.
func (d *dashboard) escutar(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// serve atende no listener dado e encerra graciosamente quando ctx cancela.
func (d *dashboard) serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: d.handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (d *dashboard) página(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func (d *dashboard) rotas(w http.ResponseWriter, r *http.Request) {
	rotas, err := d.store.listar()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rotas == nil {
		rotas = []route{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(rotas)
}

// eventos é o fluxo SSE: cada rota nova vira um "data: {...}".
func (d *dashboard) eventos(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancelar := d.broker.subscribe()
	defer cancelar()
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case rota := <-ch:
			linha, err := json.Marshal(rota)
			if err != nil {
				continue
			}
			w.Write([]byte("data: "))
			w.Write(linha)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
