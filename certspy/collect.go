package main

import (
	"net"
	"sort"
	"strings"
)

// Fontes de onde um nome pode ter vindo.
const (
	fonteCT   = "ct"
	fonteLive = "live"
)

// dominio é um nome descoberto, já deduplicado, com a prova de onde veio.
// SobApex diz se o nome pertence ao alvo; o que está fora costuma ser nome
// de terceiro que dividia certificado com o alvo — fica no relatório local
// mas, por padrão, nunca sai na publicação.
type dominio struct {
	Nome      string   `json:"nome"`
	Fontes    []string `json:"fontes"`
	NotBefore string   `json:"not_before,omitempty"`
	Vivo      bool     `json:"vivo"`
	IPs       []string `json:"ips,omitempty"`
	SobApex   bool     `json:"sob_apex"`
}

// achado é uma observação crua: um nome visto numa fonte. mesclar transforma
// muitos achados em poucos domínios.
type achado struct {
	Nome      string
	Fonte     string
	NotBefore string
}

// normalizarNome padroniza para comparar e deduplicar. O "*." sai porque o
// wildcard cobre o apex e o que importa é o nome-base; o ponto final vem de
// FQDN em alguns certificados.
func normalizarNome(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for strings.HasPrefix(s, "*.") {
		s = s[2:]
	}
	return strings.TrimRight(s, ".")
}

// nomeValido descarta o que não é hostname: e-mails (crt.sh lista os de
// certificados S/MIME), vazios e restos de wildcard no meio do nome.
func nomeValido(s string) bool {
	return s != "" && !strings.ContainsAny(s, "@* \t/")
}

// souApex diz se o nome é o próprio apex ou está abaixo dele. O ponto antes
// do apex evita que "malalvo.com" passe como se fosse subdomínio de "alvo.com".
func souApex(nome string, apexes []string) bool {
	nome = normalizarNome(nome)
	for _, a := range apexes {
		a = normalizarNome(a)
		if a != "" && (nome == a || strings.HasSuffix(nome, "."+a)) {
			return true
		}
	}
	return false
}

// mesclar deduplica os achados preservando de quais fontes cada nome veio e
// o not_before mais antigo (a primeira vez que o CT viu o nome). Datas ISO
// comparam bem como texto, sem precisar parsear formatos variáveis do crt.sh.
func mesclar(achados []achado) []dominio {
	porNome := map[string]*dominio{}
	for _, a := range achados {
		nome := normalizarNome(a.Nome)
		if !nomeValido(nome) {
			continue
		}
		d, ok := porNome[nome]
		if !ok {
			d = &dominio{Nome: nome}
			porNome[nome] = d
		}
		if !contem(d.Fontes, a.Fonte) {
			d.Fontes = append(d.Fontes, a.Fonte)
		}
		if a.NotBefore != "" && (d.NotBefore == "" || a.NotBefore < d.NotBefore) {
			d.NotBefore = a.NotBefore
		}
	}

	out := make([]dominio, 0, len(porNome))
	for _, d := range porNome {
		sort.Strings(d.Fontes)
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nome < out[j].Nome })
	return out
}

// marcarApex preenche SobApex em cada domínio.
func marcarApex(doms []dominio, apexes []string) {
	for i := range doms {
		doms[i].SobApex = souApex(doms[i].Nome, apexes)
	}
}

// apexDeHost aproxima o apex de um host pelos dois últimos rótulos. É só o
// fallback de quando não há --domain; sufixos como co.uk não são cobertos
// (mesma limitação do --scope=domain do download-dump). IP não tem apex.
func apexDeHost(host string) string {
	host = normalizarNome(host)
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	partes := strings.Split(host, ".")
	if len(partes) <= 2 {
		return host
	}
	return strings.Join(partes[len(partes)-2:], ".")
}

func contem(lista []string, v string) bool {
	for _, x := range lista {
		if x == v {
			return true
		}
	}
	return false
}
