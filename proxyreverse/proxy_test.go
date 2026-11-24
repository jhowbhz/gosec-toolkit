package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// backendFake sobe um servidor que anota o que recebeu e devolve algo
// conhecido, pra provar que o proxy repassou de verdade.
type recebido struct {
	host, method, body string
	headers            http.Header
}

func backendFake(t *testing.T, got *recebido) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.host = r.Host
		got.method = r.Method
		got.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.Header().Set("X-Backend", "sim")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "resposta-do-backend")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHandlerEncaminhaERegistra(t *testing.T) {
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	var jsonl, console bytes.Buffer
	handler, err := newHandler(cfg, newRecorder(&jsonl, &console, false))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest("POST", proxy.URL+"/api/conta", strings.NewReader(`{"acao":"saldo"}`))
	req.Host = "site.local"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request pelo proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// (a) a resposta chegou repassada, intacta
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, quero 201", resp.StatusCode)
	}
	if string(body) != "resposta-do-backend" {
		t.Errorf("body repassado = %q", string(body))
	}
	if resp.Header.Get("X-Backend") != "sim" {
		t.Errorf("header do backend não repassado")
	}
	// (b) o backend recebeu o request que mandamos
	if got.method != "POST" || got.body != `{"acao":"saldo"}` {
		t.Errorf("backend recebeu method=%q body=%q", got.method, got.body)
	}
	// por padrão o Host vai reescrito pro destino
	upstreamHost := strings.TrimPrefix(backend.URL, "http://")
	if got.host != upstreamHost {
		t.Errorf("Host recebido pelo backend = %q, quero %q (reescrito)", got.host, upstreamHost)
	}

	// (c) a troca inteira ficou registrada no JSONL
	ex := ultimaTroca(t, &jsonl)
	if ex.Status != 201 || ex.Method != "POST" {
		t.Errorf("JSONL status/method divergiu: %+v", ex)
	}
	if ex.ReqBody != `{"acao":"saldo"}` || ex.RespBody != "resposta-do-backend" {
		t.Errorf("JSONL não guardou os corpos: req=%q resp=%q", ex.ReqBody, ex.RespBody)
	}
	if !strings.Contains(ex.URL, "/api/conta") {
		t.Errorf("JSONL URL sem o caminho: %q", ex.URL)
	}
}

func TestHandlerPreserveHostMantemOriginal(t *testing.T) {
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL, "--preserve-host"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	handler, err := newHandler(cfg, newRecorder(io.Discard, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest("GET", proxy.URL+"/", nil)
	req.Host = "site.local"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if got.host != "site.local" {
		t.Errorf("com --preserve-host o backend deveria ver Host=site.local, viu %q", got.host)
	}
}

func TestHandlerRegistraFalhaDeUpstream(t *testing.T) {
	// upstream morto: o proxy devolve 502 e registra a falha (pentest vê o erro).
	cfg, err := parseConfig([]string{"--from=site.local", "--to=http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	var jsonl bytes.Buffer
	handler, err := newHandler(cfg, newRecorder(&jsonl, io.Discard, false))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/x")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, quero 502", resp.StatusCode)
	}
	ex := ultimaTroca(t, &jsonl)
	if ex.Error == "" {
		t.Errorf("JSONL deveria registrar o erro do upstream: %+v", ex)
	}
}

func TestHandlerRecusaRequestEmLoop(t *testing.T) {
	// Um request que já carrega a marca do proxy significa que o --to apontou
	// de volta pra cá: recusa com 508 e NÃO repassa (quebra a amplificação).
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	var jsonl bytes.Buffer
	handler, err := newHandler(cfg, newRecorder(&jsonl, io.Discard, false))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest("GET", proxy.URL+"/", nil)
	req.Header.Set(loopHeader, "1") // simula request que já passou por aqui
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusLoopDetected {
		t.Errorf("status = %d, quero 508 (loop detected)", resp.StatusCode)
	}
	if got.method != "" {
		t.Errorf("loop NÃO deveria chegar no backend; backend viu method=%q", got.method)
	}
	ex := ultimaTroca(t, &jsonl)
	if ex.Error == "" {
		t.Errorf("loop deveria virar registro com erro: %+v", ex)
	}
}

func TestHandlerUpstreamAddrDiscaEnderecoFixoMantemHost(t *testing.T) {
	// O caso "todo tráfego pelo proxy": o nome aponta pra 127.0.0.1 no hosts,
	// então --to não resolve pra origem real. --upstream-addr disca o IP real
	// (aqui, o backend de teste) mantendo o Host do --to.
	var got recebido
	backend := backendFake(t, &got)
	backendAddr := strings.TrimPrefix(backend.URL, "http://") // 127.0.0.1:porta

	cfg, err := parseConfig([]string{
		"--from=meusite.com.br",
		"--to=http://origem-real.invalid", // host que NÃO resolve
		"--upstream-addr=" + backendAddr,  // mas disca aqui
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	handler, err := newHandler(cfg, newRecorder(io.Discard, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/api/conta")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	// chegou no backend apesar de --to ser .invalid => discou o endereço fixo
	if got.method != "GET" {
		t.Errorf("não discou pro --upstream-addr; backend viu method=%q", got.method)
	}
	// o Host continua sendo o do --to (a origem lógica), não o IP discado
	if got.host != "origem-real.invalid" {
		t.Errorf("Host recebido = %q, quero origem-real.invalid (do --to, não o IP)", got.host)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, quero 201", resp.StatusCode)
	}
}

func TestHandlerSempreIdentificaAFerramenta(t *testing.T) {
	// "se eu não passar nada, envia do mesmo jeito": sem --header, o destino
	// ainda recebe o header que identifica a ferramenta de pentest.
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	handler, err := newHandler(cfg, newRecorder(io.Discard, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if got.headers.Get("X-Proxyreverse") == "" {
		t.Error("sem --header, o destino deveria receber o header de identificação da ferramenta")
	}
}

func TestHandlerAdicionaHeadersDoUsuario(t *testing.T) {
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL, "--header", "X-Pentest: autorizado"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	handler, err := newHandler(cfg, newRecorder(io.Discard, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if got.headers.Get("X-Pentest") != "autorizado" {
		t.Errorf("header do usuário não chegou no destino: %q", got.headers.Get("X-Pentest"))
	}
	if got.headers.Get("X-Proxyreverse") == "" {
		t.Error("o header de identificação deveria continuar indo junto com os do usuário")
	}
}

func TestHandlerUsuarioPodeSobrescreverIdent(t *testing.T) {
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL, "--header", "X-Proxyreverse: meu-valor"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	handler, err := newHandler(cfg, newRecorder(io.Discard, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if got.headers.Get("X-Proxyreverse") != "meu-valor" {
		t.Errorf("--header deveria sobrescrever o ident; veio %q", got.headers.Get("X-Proxyreverse"))
	}
}

func TestLogRegistraHeadersEnviadosAoDestino(t *testing.T) {
	// O log deve refletir o que o DESTINO recebeu, incluindo o X-Proxyreverse
	// e os --header que o proxy adiciona — não só os headers de entrada.
	var got recebido
	backend := backendFake(t, &got)

	cfg, err := parseConfig([]string{"--from=site.local", "--to=" + backend.URL, "--header", "X-Pentest: autorizado"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	var jsonl bytes.Buffer
	handler, err := newHandler(cfg, newRecorder(&jsonl, io.Discard, true))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	ex := ultimaTroca(t, &jsonl)
	if ex.ReqHeaders.Get("X-Proxyreverse") == "" {
		t.Error("o log deveria mostrar o X-Proxyreverse enviado ao destino")
	}
	if ex.ReqHeaders.Get("X-Pentest") != "autorizado" {
		t.Errorf("o log deveria mostrar o --header enviado ao destino; veio %q", ex.ReqHeaders.Get("X-Pentest"))
	}
}

func ultimaTroca(t *testing.T, jsonl *bytes.Buffer) exchange {
	t.Helper()
	linhas := strings.Split(strings.TrimSpace(jsonl.String()), "\n")
	if len(linhas) == 0 || linhas[0] == "" {
		t.Fatal("nenhuma troca registrada no JSONL")
	}
	var ex exchange
	if err := json.Unmarshal([]byte(linhas[len(linhas)-1]), &ex); err != nil {
		t.Fatalf("última linha do JSONL não é JSON: %v", err)
	}
	return ex
}
