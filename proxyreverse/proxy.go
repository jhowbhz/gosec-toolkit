package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"time"
)

// identHeader marca, pro destino, qual ferramenta está no meio — enviado
// em toda request (teste autorizado, previsto no destino).
const identHeader = "X-Proxyreverse"

// ctxChave identifica os dados da troca carregados no contexto do request.
// Tipo privado pra não colidir com chaves de outros pacotes.
type ctxChave struct{}

// reqMeta viaja no contexto do request pra que a resposta (ou o erro) do
// upstream seja montada como UMA troca, correlacionada com o que entrou.
type reqMeta struct {
	id        string
	start     time.Time
	method    string
	url       string
	body      string
	truncated bool
}

// newHandler monta o proxy reverso: bufferiza o request pra registrar,
// repassa pro upstream reescrevendo Host/scheme, e registra a resposta
// (ou a falha) como uma troca completa.
//
// forma aqui, rede lá: a decisão de PRA ONDE e COMO repassar mora neste
// arquivo; quem só grava o que passou é o recorder.
func newHandler(c Config, rec *recorder) (http.Handler, error) {
	upstream, err := c.upstreamURL()
	if err != nil {
		return nil, err
	}
	maxBody := c.maxBodyBytes()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure},
	}
	// --upstream-addr: disca sempre esse endereço, ignorando pra onde o nome
	// do --to resolveria. O TLS (SNI) e o header Host continuam vindo do --to,
	// então a origem atrás de CDN recebe o Host certo mesmo discando o IP.
	// É o que permite "todo tráfego pelo proxy" quando o nome aponta pra
	// 127.0.0.1 no hosts: o nome cai aqui, e daqui vai pro IP real.
	if c.UpstreamAddr != "" {
		dialer := &net.Dialer{}
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, c.UpstreamAddr)
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream) // scheme + host + junção de caminho
			if c.PreserveHost {
				pr.Out.Host = pr.In.Host // mantém o host interceptado
			} else {
				pr.Out.Host = upstream.Host // o que o backend/vhost espera
			}
			pr.SetXForwarded()
			marcarPassagem(pr.Out.Header) // pra detectar se voltar pra cá
			// identifica a ferramenta pro destino SEMPRE (mesmo sem --header):
			// é um teste autorizado, previsto no destino.
			pr.Out.Header.Set(identHeader, "proxyreverse/"+version)
			for _, h := range c.Headers {
				pr.Out.Header.Set(h.Name, h.Value) // extras do usuário; podem sobrescrever o ident
			}
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			meta, _ := resp.Request.Context().Value(ctxChave{}).(*reqMeta)
			respFull, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respFull))
			respLog, respTrunc := bodyForLog(respFull, maxBody)
			rec.record(trocaDe(meta, resp.Request.Header.Clone(), resp.StatusCode, resp.Header.Clone(), respLog, respTrunc, ""))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			meta, _ := r.Context().Value(ctxChave{}).(*reqMeta)
			rec.record(trocaDe(meta, r.Header.Clone(), 0, nil, "", false, err.Error()))
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := &reqMeta{
			id:     newID(),
			start:  time.Now(),
			method: r.Method,
			url:    urlLogico(r),
		}
		// Request que já passou por aqui = --to apontou de volta pro proxy.
		// Recusa com 508 em vez de repassar: é o que quebra a amplificação.
		if jaPassouPorAqui(r.Header) {
			rec.record(trocaDe(meta, r.Header.Clone(), http.StatusLoopDetected, nil, "", false,
				"loop detectado: request já passou por este proxy (--to aponta de volta pra cá?)"))
			w.WriteHeader(http.StatusLoopDetected)
			return
		}
		// Bufferiza o corpo da entrada: lê inteiro pra repassar, guarda só
		// o teto pro log. Isto não faz streaming de upload gigante — é um
		// proxy de inspeção, o corpo cabe em memória de propósito.
		if r.Body != nil {
			reqFull, _ := io.ReadAll(r.Body)
			r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(reqFull))
			meta.body, meta.truncated = bodyForLog(reqFull, maxBody)
		}
		ctx := context.WithValue(r.Context(), ctxChave{}, meta)
		proxy.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}

// trocaDe monta o registro da troca a partir do meta do request e do
// desfecho (resposta ou erro). Tolera meta nil pra não derrubar o log.
// trocaDe monta o registro. reqHeaders são os headers REALMENTE enviados ao
// destino (já com X-Proxyreverse, --header e X-Forwarded-*), não os de entrada
// — é o que importa inspecionar num proxy de interceptação.
func trocaDe(meta *reqMeta, reqHeaders http.Header, status int, respHeaders http.Header, respBody string, respTrunc bool, errMsg string) exchange {
	ex := exchange{
		ReqHeaders:        reqHeaders,
		Status:            status,
		RespHeaders:       respHeaders,
		RespBody:          respBody,
		RespBodyTruncated: respTrunc,
		Error:             errMsg,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
	}
	if meta != nil {
		ex.ID = meta.id
		ex.Method = meta.method
		ex.URL = meta.url
		ex.ReqBody = meta.body
		ex.ReqBodyTruncated = meta.truncated
		ex.Timestamp = meta.start.UTC().Format(time.RFC3339)
		ex.DurationMS = time.Since(meta.start).Milliseconds()
	}
	return ex
}

// urlLogico reconstrói a URL que o cliente pediu (o host interceptado),
// não a do upstream — é isso que faz sentido no registro do pentest.
func urlLogico(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}
