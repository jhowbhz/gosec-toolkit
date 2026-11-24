package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Motivos de parada, mostrados no resumo para que fique claro por que o
// teste acabou — sobretudo quando foi o primeiro 429 e não o total.
const (
	motivoTotal      = "total de requests atingido"
	motivoDuracao    = "duração máxima atingida"
	motivo429        = "parou no primeiro 429 (stop-on-429)"
	motivoInterrompe = "interrompido (Ctrl+C)"
)

// limiteLeituraCorpo basta para drenar a resposta e liberar a conexão; o
// conteúdo não interessa, só status e headers.
const limiteLeituraCorpo = 1 << 20

// meter envia a sequência controlada de requests idênticos e entrega cada
// resultado ao recorder.
type meter struct {
	cfg     Config
	rec     *recorder
	client  *http.Client
	headers http.Header
	corpo   []byte
	inicio  time.Time
	parar   atomic.Bool // ligado quando um 429 manda parar (stop-on-429)
}

func newMeter(cfg Config, rec *recorder) (*meter, error) {
	headers, err := cfg.parsedHeaders()
	if err != nil {
		return nil, err
	}
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", "ratemeter/1.0 (+https://github.com/jhowbhz/ratemeter)")
	}

	corpo := []byte(cfg.Body)
	if cfg.BodyFile != "" {
		if corpo, err = os.ReadFile(cfg.BodyFile); err != nil {
			return nil, fmt.Errorf("não consegui ler --body-file %s: %w", cfg.BodyFile, err)
		}
	}

	client := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
		// Redirect seguido esconderia o status real do endpoint medido.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if cfg.Insecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	return &meter{cfg: cfg, rec: rec, client: client, headers: headers, corpo: corpo}, nil
}

// taxaDoPasso diz a taxa (req/s) da vez. Sem rampa é constante. Com rampa o
// total é dividido em degraus iguais e a taxa sobe de ~rate/steps até rate,
// nunca abaixo de 1/s; assim o degrau em que o 429 aparece fica legível.
func taxaDoPasso(seq, total int, rampa bool, passos, rate int) int {
	if !rampa {
		return rate
	}
	passo := (seq - 1) * passos / total
	if passo >= passos {
		passo = passos - 1
	}
	return max(1, rate*(passo+1)/passos)
}

// run dispara os requests no ritmo do ticker até total, duração, primeiro 429
// (se stop-on-429) ou Ctrl+C. Devolve o motivo da parada. Os requests em voo
// terminam antes de retornar, para o resumo não perder resposta.
func (m *meter) run(ctx context.Context) string {
	agenda := ctx
	if m.cfg.Duration > 0 {
		var cancel context.CancelFunc
		agenda, cancel = context.WithTimeout(ctx, time.Duration(m.cfg.Duration)*time.Second)
		defer cancel()
	}

	m.inicio = time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	vagas := make(chan struct{}, m.cfg.Concurrency)
	var wg sync.WaitGroup
	motivo := motivoTotal
	taxaAtual := 0

laco:
	for seq := 1; seq <= m.cfg.Total; seq++ {
		taxa := taxaDoPasso(seq, m.cfg.Total, m.cfg.Ramp, m.cfg.RampSteps, m.cfg.Rate)
		if taxa != taxaAtual {
			ticker.Reset(time.Second / time.Duration(taxa))
			taxaAtual = taxa
		}
		if seq > 1 { // o primeiro sai na hora; os demais esperam o ritmo
			select {
			case <-agenda.Done():
				motivo = motivoDaAgenda(ctx)
				break laco
			case <-ticker.C:
			}
		}
		if m.parar.Load() {
			break
		}
		select {
		case vagas <- struct{}{}:
		case <-agenda.Done():
			motivo = motivoDaAgenda(ctx)
			break laco
		}
		wg.Add(1)
		go func(seq int) {
			defer wg.Done()
			defer func() { <-vagas }()
			m.enviar(ctx, seq)
		}(seq)
	}
	wg.Wait()

	if m.parar.Load() {
		return motivo429
	}
	return motivo
}

// motivoDaAgenda separa Ctrl+C (o contexto pai caiu) de duração esgotada.
func motivoDaAgenda(pai context.Context) string {
	if pai.Err() != nil {
		return motivoInterrompe
	}
	return motivoDuracao
}

// enviar faz um request e registra o resultado. O contexto do request é o
// pai, não o da agenda: a duração máxima corta novos envios, mas não aborta
// quem já está em voo.
func (m *meter) enviar(ctx context.Context, seq int) {
	reg := registro{Seq: seq, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	inicio := time.Now()
	reg.ElapsedMS = inicio.Sub(m.inicio).Milliseconds()

	req, err := http.NewRequestWithContext(ctx, m.cfg.Method, m.cfg.URL, bytes.NewReader(m.corpo))
	if err != nil {
		reg.Error = err.Error()
		m.rec.record(reg)
		return
	}
	req.Header = m.headers.Clone()

	resp, err := m.client.Do(req)
	reg.DurationMS = time.Since(inicio).Milliseconds()
	if err != nil {
		reg.Error = err.Error()
		m.rec.record(reg)
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, limiteLeituraCorpo))
	resp.Body.Close()

	lim := parseRateLimitHeaders(resp.Header)
	reg.Status = resp.StatusCode
	reg.RetryAfter = lim.RetryAfter
	reg.RateLimitLimit = lim.Limit
	reg.RateLimitRemaining = lim.Remaining
	reg.RateLimitReset = lim.Reset
	m.rec.record(reg)

	if resp.StatusCode == http.StatusTooManyRequests && m.cfg.StopOn429 {
		m.parar.Store(true)
	}
}
