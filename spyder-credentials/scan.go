package main

import (
	"math"
	"regexp"
	"strings"
)

// classe diz o que um achado representa, que é o eixo que realmente importa
// num relatório de segredos: um valor que JAMAIS deveria vazar (secreto), um
// que é público por desenho — ex.: chave publishable do Supabase, pk_live do
// Stripe — (publicavel), ou algo que parece segredo e precisa de olho humano
// (suspeito).
type classe string

const (
	classeSecreto    classe = "secreto"
	classePublicavel classe = "publicavel"
	classeSuspeito   classe = "suspeito"
)

// Níveis de severidade exibidos no relatório.
const (
	sevCritico = "crítico"
	sevAlto    = "alto"
	sevMedio   = "médio"
	sevInfo    = "informativo"
)

// achado é uma ocorrência de credencial/segredo encontrada. Valor guarda o
// segredo cru; o mascaramento é feito só na saída (ver redact.go).
type achado struct {
	Detector    string   `json:"detector"`
	Classe      classe   `json:"classe"`
	Severidade  string   `json:"severidade"`
	Valor       string   `json:"-"`
	Preview     string   `json:"preview"`
	Local       string   `json:"local"`
	Linha       int      `json:"linha"`
	Extra       string   `json:"extra,omitempty"`
	Ocorrencias int      `json:"ocorrencias,omitempty"`
	Locais      []string `json:"locais,omitempty"`
}

// detector é uma regra de busca. refina (quando presente) decide a partir do
// texto casado a severidade, a classe, um comentário e se o achado vale —
// usado para classificar JWT por role e para descartar placeholders.
type detector struct {
	nome   string
	re     *regexp.Regexp
	sev    string
	classe classe
	refina func(val, full string) (sev string, cl classe, extra string, manter bool)
}

// classificar aplica refina ou cai no padrão fixo do detector.
func (d detector) classificar(val, full string) (string, classe, string, bool) {
	if d.refina != nil {
		return d.refina(val, full)
	}
	return d.sev, d.classe, "", true
}

// varrer roda todos os detectores sobre um texto e devolve os achados, com o
// número da linha de cada ocorrência. entropia liga o detector genérico de
// alta entropia (ruidoso), que fica desligado por padrão.
func varrer(texto, local string, dets []detector, entropia bool) []achado {
	var out []achado
	for _, d := range dets {
		for _, loc := range d.re.FindAllStringSubmatchIndex(texto, -1) {
			full := texto[loc[0]:loc[1]]
			val := full
			if len(loc) >= 4 && loc[2] >= 0 { // grupo 1, quando o detector captura um
				val = texto[loc[2]:loc[3]]
			}
			sev, cl, extra, manter := d.classificar(val, full)
			if !manter {
				continue
			}
			out = append(out, achado{
				Detector: d.nome, Classe: cl, Severidade: sev,
				Valor: val, Local: local, Linha: linhaDoOffset(texto, loc[0]), Extra: extra,
			})
		}
	}
	if entropia {
		out = append(out, varrerEntropia(texto, local)...)
	}
	return out
}

// linhaDoOffset devolve o número da linha (1-based) de um deslocamento.
func linhaDoOffset(texto string, off int) int {
	return 1 + strings.Count(texto[:off], "\n")
}

var reEntropia = regexp.MustCompile(`[A-Za-z0-9+/_-]{24,}`)

// varrerEntropia procura cadeias longas de alta entropia (possível chave/token
// sem formato conhecido). É deliberadamente conservador para não encher de
// ruído: só aceita entropia de Shannon acima de 4 bits/caractere.
func varrerEntropia(texto, local string) []achado {
	var out []achado
	for _, loc := range reEntropia.FindAllStringIndex(texto, -1) {
		s := texto[loc[0]:loc[1]]
		if entropiaShannon(s) < 4.0 {
			continue
		}
		out = append(out, achado{
			Detector: "entropia-alta", Classe: classeSuspeito, Severidade: sevMedio,
			Valor: s, Local: local, Linha: linhaDoOffset(texto, loc[0]),
			Extra: "cadeia de alta entropia (sem formato conhecido)",
		})
	}
	return out
}

// entropiaShannon calcula a entropia de Shannon em bits por caractere.
func entropiaShannon(s string) float64 {
	if s == "" {
		return 0
	}
	freq := map[rune]float64{}
	for _, r := range s {
		freq[r]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range freq {
		p := c / n
		h -= p * math.Log2(p)
	}
	return h
}
