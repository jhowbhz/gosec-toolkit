package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// loopHeader marca um request que já passou por este proxy. Se um request
// chega com essa marca, é porque o --to apontou de volta pra cá — sem o
// guarda, isso vira amplificação infinita até esgotar as portas da máquina.
const loopHeader = "X-Proxyreverse-Loop"

// marcarPassagem carimba o request de saída.
func marcarPassagem(h http.Header) { h.Set(loopHeader, "1") }

// jaPassouPorAqui diz se o request que chegou já carrega a marca.
func jaPassouPorAqui(h http.Header) bool { return h.Get(loopHeader) != "" }

// checarLoopDireto recusa, já no startup, o loop óbvio: --to apontando pra
// um endereço de loopback na mesma porta que o proxy escuta. É a verdade
// fresca que a validação de flag não tinha como saber (depende de qual
// porta vai subir). O guarda por request cobre os loops menos óbvios
// (ex.: --to com um hostname que o hosts resolve pra 127.0.0.1).
func checarLoopDireto(cfg Config) error {
	u, err := cfg.upstreamURL()
	if err != nil {
		return err
	}
	if !hostEhLoopback(u.Hostname()) {
		return nil
	}
	toPort := u.Port()
	if toPort == "" {
		toPort = portaPadrao(u.Scheme)
	}

	httpAddr, httpsAddr := cfg.listenAddrs()
	for _, addr := range []string{httpAddr, httpsAddr} {
		if addr == "" {
			continue
		}
		_, p, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		if p == toPort {
			return fmt.Errorf(
				"--to=%s aponta pra este próprio proxy (loopback na porta %s, que você está escutando): isso é um loop. "+
					"Use o IP ou host REAL do destino, não 127.0.0.1/localhost na mesma porta",
				cfg.To, toPort)
		}
	}
	return nil
}

func hostEhLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func portaPadrao(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}
