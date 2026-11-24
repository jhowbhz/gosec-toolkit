package main

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// robots é o robots.txt já resolvido para o nosso User-Agent: só as regras
// do grupo que se aplica a nós e o Crawl-delay dele. É deliberadamente um
// subconjunto do protocolo (User-agent, Allow, Disallow, Crawl-delay, com
// curinga * e âncora $) — o suficiente para rastrear com educação.
type robots struct {
	rules []robotsRule
	delay time.Duration
}

// robotsRule é uma linha Allow/Disallow já traduzida: o regex que casa o
// caminho e o comprimento do padrão literal, que decide a especificidade.
type robotsRule struct {
	allow bool
	re    *regexp.Regexp
	peso  int
}

// grupo acumula as linhas User-agent e as regras até o próximo bloco.
type grupo struct {
	agentes  []string
	rules    []robotsRule
	delay    time.Duration
	temDelay bool
}

// parseRobots interpreta o corpo do robots.txt e escolhe o grupo que vale
// para userAgent: o grupo específico mais longo que casa, ou o curinga (*).
// Corpo vazio ou sem grupo aplicável = permite tudo.
func parseRobots(body []byte, userAgent string) *robots {
	grupos := lerGrupos(body)
	g := escolherGrupo(grupos, strings.ToLower(userAgent))
	if g == nil {
		return &robots{}
	}
	return &robots{rules: g.rules, delay: g.delay}
}

// lerGrupos quebra o arquivo em grupos. Uma linha User-agent após uma linha
// de regra inicia um grupo novo; User-agents consecutivos compartilham o
// mesmo grupo (vários agentes, mesmas regras).
func lerGrupos(body []byte) []grupo {
	var grupos []grupo
	var atual *grupo
	esperandoAgente := false

	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		campo, valor, ok := campoLinha(sc.Text())
		if !ok {
			continue
		}
		switch campo {
		case "user-agent":
			if atual == nil || !esperandoAgente {
				grupos = append(grupos, grupo{})
				atual = &grupos[len(grupos)-1]
			}
			atual.agentes = append(atual.agentes, strings.ToLower(valor))
			esperandoAgente = true
		case "disallow", "allow":
			if atual == nil {
				continue // regra órfã, sem User-agent antes: ignora
			}
			esperandoAgente = false
			if campo == "disallow" && valor == "" {
				continue // Disallow vazio = sem restrição
			}
			atual.rules = append(atual.rules, compilarRegra(campo == "allow", valor))
		case "crawl-delay":
			if atual == nil {
				continue
			}
			esperandoAgente = false
			if seg, err := strconv.ParseFloat(valor, 64); err == nil && seg >= 0 {
				atual.delay = time.Duration(seg * float64(time.Second))
				atual.temDelay = true
			}
		}
	}
	return grupos
}

// campoLinha separa "campo: valor", tira comentário (#) e normaliza o campo
// para minúsculas. Devolve ok=false para linha em branco ou sem ':'.
func campoLinha(linha string) (campo, valor string, ok bool) {
	if i := strings.IndexByte(linha, '#'); i >= 0 {
		linha = linha[:i]
	}
	campo, valor, ok = strings.Cut(linha, ":")
	if !ok {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(campo)), strings.TrimSpace(valor), true
}

// escolherGrupo pega o grupo cujo agente é o prefixo mais longo que casa com
// o nosso UA; sem específico, cai no curinga (*). Empate de comprimento fica
// com o primeiro encontrado.
func escolherGrupo(grupos []grupo, uaLower string) *grupo {
	var melhor *grupo
	melhorLen := -1
	var curinga *grupo
	for i := range grupos {
		for _, ag := range grupos[i].agentes {
			if ag == "*" {
				if curinga == nil {
					curinga = &grupos[i]
				}
				continue
			}
			if strings.Contains(uaLower, ag) && len(ag) > melhorLen {
				melhor = &grupos[i]
				melhorLen = len(ag)
			}
		}
	}
	if melhor != nil {
		return melhor
	}
	return curinga
}

// compilarRegra traduz um padrão de path (com * e $) num regex ancorado no
// início, e guarda o comprimento literal como peso de especificidade.
func compilarRegra(allow bool, padrao string) robotsRule {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range padrao {
		switch r {
		case '*':
			b.WriteString(".*")
		case '$':
			b.WriteString("$")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		// Padrão impossível de compilar vira regra que nunca casa.
		re = regexp.MustCompile(`$.^`)
	}
	return robotsRule{allow: allow, re: re, peso: len(padrao)}
}

// allowed decide se o caminho pode ser rastreado. Regra que casa com maior
// peso vence; empate fica com Allow (menos restritivo), como manda a prática.
func (r *robots) allowed(path string) bool {
	if path == "" {
		path = "/"
	}
	melhorPeso := -1
	permitido := true
	for _, rule := range r.rules {
		if !rule.re.MatchString(path) {
			continue
		}
		if rule.peso > melhorPeso || (rule.peso == melhorPeso && rule.allow) {
			melhorPeso = rule.peso
			permitido = rule.allow
		}
	}
	return permitido
}

// crawlDelay é a pausa mínima por host pedida pelo robots (0 se não houver).
func (r *robots) crawlDelay() time.Duration {
	return r.delay
}
