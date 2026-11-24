package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// payloadPublicacao é o corpo do POST. Os alvos vão junto para o receptor
// saber a que se refere a lista.
type payloadPublicacao struct {
	Target      []string  `json:"target"`
	GeneratedAt string    `json:"generated_at"`
	Domains     []dominio `json:"domains"`
}

// montarPayload decide o que sai da máquina. Por padrão só nomes sob o apex
// do alvo: nome de terceiro que dividia certificado com o alvo não é nosso
// para publicar. --publish-all é a escolha explícita de abrir mão disso.
func montarPayload(alvo []string, doms []dominio, todos bool, quando time.Time) payloadPublicacao {
	p := payloadPublicacao{
		Target:      alvo,
		GeneratedAt: quando.UTC().Format(time.RFC3339),
		Domains:     []dominio{},
	}
	for _, d := range doms {
		if todos || d.SobApex {
			p.Domains = append(p.Domains, d)
		}
	}
	return p
}

// publicar faz o POST. Roda depois das saídas locais gravadas, então falha
// aqui nunca custa o resultado da varredura.
func publicar(ctx context.Context, cfg Config, p payloadPublicacao) error {
	corpo, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.PublishURL, bytes.NewReader(corpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", cfg.UserAgent)
	if cfg.PublishToken != "" {
		req.Header.Set("Authorization", cfg.PublishToken)
	}

	cli := &http.Client{
		Timeout:   time.Duration(cfg.TimeoutS) * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure}},
	}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("publish: o destino respondeu status %d", resp.StatusCode)
	}
	return nil
}
