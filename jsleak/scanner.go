package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultUserAgent = "jsleak/1.0 (+https://github.com/jhowbhz/jsleak)"
	// visiveisRedacao é quantos chars do começo do segredo sobrevivem à
	// redação: o bastante para o dono reconhecer o prefixo (AKIA, ghp_) e
	// localizar a credencial, pouco demais para reaproveitá-la.
	visiveisRedacao = 4
	// bytesSondaBinario é o trecho inicial inspecionado atrás de NUL para
	// decidir que o arquivo é binário (regex em binário só gera ruído).
	bytesSondaBinario = 8000
)

// varredor aplica o catálogo às fontes pedidas e entrega os achados ao
// recorder. Só LÊ: nada do que encontra sai da máquina além do que o
// operador mandou baixar com --url.
type varredor struct {
	cfg     Config
	rec     *recorder
	regras  []regra
	exts    map[string]bool
	headers http.Header
	client  *http.Client
}

func newVarredor(cfg Config, rec *recorder) (*varredor, error) {
	headers, err := cfg.parsedHeaders()
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &varredor{
		cfg:     cfg,
		rec:     rec,
		regras:  catalogo(),
		exts:    cfg.extensoes(),
		headers: headers,
		client: &http.Client{
			Timeout:   time.Duration(cfg.TimeoutSec) * time.Second,
			Transport: transport,
		},
	}, nil
}

// run varre todas as fontes na ordem: diretórios, arquivos, URLs. Cancelar
// o contexto (Ctrl+C) interrompe entre uma fonte e outra.
func (v *varredor) run(ctx context.Context) {
	for _, d := range v.cfg.Dirs {
		v.varrerDir(ctx, d)
	}
	for _, f := range v.cfg.Files {
		if ctx.Err() != nil {
			return
		}
		v.varrerArquivo(f)
	}
	for _, u := range v.cfg.URLs {
		if ctx.Err() != nil {
			return
		}
		v.varrerURL(ctx, u)
	}
}

// varrerDir caminha o diretório e filtra por extensão. O filtro vale só aqui:
// um --file é pedido explícito do operador e é varrido qualquer que seja o nome.
func (v *varredor) varrerDir(ctx context.Context, raiz string) {
	err := filepath.WalkDir(raiz, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			v.rec.erro("file:"+path, err)
			return nil // um item ilegível não deve abortar o resto da árvore
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if !v.exts[ext] {
			return nil
		}
		v.varrerArquivo(path)
		return nil
	})
	if err != nil && ctx.Err() == nil {
		v.rec.erro("file:"+raiz, err)
	}
}

func (v *varredor) varrerArquivo(path string) {
	source := "file:" + path
	info, err := os.Stat(path)
	if err != nil {
		v.rec.erro(source, err)
		return
	}
	if max := v.cfg.maxBodyBytes(); max > 0 && info.Size() > max {
		return // acima do teto pedido em --max-body
	}
	body, err := os.ReadFile(path)
	if err != nil {
		v.rec.erro(source, err)
		return
	}
	v.varrerConteudo(source, body)
}

// varrerURL faz um GET passivo (sem seguir links, sem POST) e varre o corpo.
func (v *varredor) varrerURL(ctx context.Context, alvo string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, alvo, nil)
	if err != nil {
		v.rec.erro(alvo, err)
		return
	}
	req.Header = v.headers.Clone()
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		v.rec.erro(alvo, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		v.rec.erro(alvo, fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var leitor io.Reader = resp.Body
	max := v.cfg.maxBodyBytes()
	if max > 0 {
		leitor = io.LimitReader(resp.Body, max+1) // +1 para distinguir "exato" de "estourou"
	}
	body, err := io.ReadAll(leitor)
	if err != nil {
		v.rec.erro(alvo, err)
		return
	}
	if max > 0 && int64(len(body)) > max {
		return
	}
	v.varrerConteudo(alvo, body)
}

// varrerConteudo aplica o catálogo linha a linha. Linha a linha basta até
// para chave privada, porque o catálogo casa só o cabeçalho BEGIN, que
// ocupa uma linha; e mantém o número da linha no achado.
func (v *varredor) varrerConteudo(source string, body []byte) {
	if eBinario(body) {
		return
	}
	v.rec.fonteVarrida()
	for i, linha := range strings.Split(string(body), "\n") {
		linha = strings.TrimRight(linha, "\r")
		for _, t := range analisarLinha(linha, v.regras, v.cfg.ContextChars, v.cfg.Redact) {
			v.rec.record(achado{
				Source:   source,
				Line:     i + 1,
				Rule:     t.Regra.Nome,
				Severity: t.Regra.Severidade,
				Match:    t.Match,
				Context:  t.Contexto,
				Note:     t.Regra.Nota,
			})
		}
	}
}

func eBinario(body []byte) bool {
	sonda := body
	if len(sonda) > bytesSondaBinario {
		sonda = sonda[:bytesSondaBinario]
	}
	return bytes.IndexByte(sonda, 0) >= 0
}

// trecho é um casamento de uma regra numa linha, já pronto para virar achado.
type trecho struct {
	Regra    regra
	Match    string
	Contexto string
}

type intervalo struct{ ini, fim int }

// analisarLinha aplica todas as regras a uma linha. Com redact, o contexto
// sai de uma cópia da linha em que TODOS os casamentos do catálogo já foram
// redigidos: do contrário o contexto de um achado vazaria o segredo vizinho
// (ex.: dois tokens na mesma linha de JS minificado).
func analisarLinha(linha string, regras []regra, ctxChars int, redact bool) []trecho {
	type casamento struct {
		intervalo
		regra regra
	}
	var casados []casamento
	for _, r := range regras {
		for _, loc := range r.Re.FindAllStringIndex(linha, -1) {
			casados = append(casados, casamento{intervalo{loc[0], loc[1]}, r})
		}
	}
	if len(casados) == 0 {
		return nil
	}
	sort.SliceStable(casados, func(i, j int) bool { return casados[i].ini < casados[j].ini })

	// Funde sobreposições: regras diferentes casam o mesmo segredo (um JWT
	// também é Bearer), e redigir duas vezes o mesmo trecho o corromperia.
	var fundidos []intervalo
	for _, c := range casados {
		if n := len(fundidos); n > 0 && c.ini < fundidos[n-1].fim {
			if c.fim > fundidos[n-1].fim {
				fundidos[n-1].fim = c.fim
			}
			continue
		}
		fundidos = append(fundidos, c.intervalo)
	}

	redigida := linha
	novaPosicao := make([]intervalo, len(fundidos))
	if redact {
		var b strings.Builder
		pos := 0
		for i, f := range fundidos {
			b.WriteString(linha[pos:f.ini])
			novaPosicao[i].ini = b.Len()
			b.WriteString(redigir(linha[f.ini:f.fim]))
			novaPosicao[i].fim = b.Len()
			pos = f.fim
		}
		b.WriteString(linha[pos:])
		redigida = b.String()
	}

	out := make([]trecho, 0, len(casados))
	for _, c := range casados {
		match := linha[c.ini:c.fim]
		if !redact {
			out = append(out, trecho{c.regra, match, janela(linha, c.ini, c.fim, ctxChars)})
			continue
		}
		for i, f := range fundidos {
			if c.ini >= f.ini && c.fim <= f.fim {
				out = append(out, trecho{c.regra, redigir(match),
					janela(redigida, novaPosicao[i].ini, novaPosicao[i].fim, ctxChars)})
				break
			}
		}
	}
	return out
}

// janela devolve o trecho [ini,fim) com n bytes de contexto de cada lado,
// recuando para fronteira de rune para não cortar UTF-8 ao meio.
func janela(s string, ini, fim, n int) string {
	a := ini - n
	if a < 0 {
		a = 0
	}
	for a < ini && !utf8.RuneStart(s[a]) {
		a++
	}
	b := fim + n
	if b > len(s) {
		b = len(s)
	}
	for b > fim && b < len(s) && !utf8.RuneStart(s[b]) {
		b--
	}
	return strings.TrimSpace(s[a:b])
}

// redigir mantém só o começo do segredo e troca o resto por "…[N]", onde N é
// quantos chars foram escondidos. Segredo curto mostra menos (um quarto) para
// que o prefixo visível nunca seja metade do valor. Função pura: é o que
// garante que o relatório não carrega a credencial inteira.
func redigir(s string) string {
	r := []rune(s)
	visiveis := visiveisRedacao
	if len(r) <= 2*visiveis {
		visiveis = len(r) / 4
	}
	return fmt.Sprintf("%s…[%d]", string(r[:visiveis]), len(r)-visiveis)
}
