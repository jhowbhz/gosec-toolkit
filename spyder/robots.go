package main

import (
	"strconv"
	"strings"
	"time"
)

// robotsPolicy é a decisão de crawl pro nosso User-Agent, extraída de um
// robots.txt. Sem regras aplicáveis, libera tudo.
type robotsPolicy struct {
	disallow   []string
	allow      []string
	crawlDelay time.Duration
}

type robotsGroup struct {
	uas        []string
	disallow   []string
	allow      []string
	crawlDelay time.Duration
}

// parseRobots lê um robots.txt e devolve a política pro User-Agent dado.
// Escolhe o grupo que casa com o nosso UA; se nenhum casar, usa o grupo "*".
func parseRobots(body, ua string) *robotsPolicy {
	var groups []robotsGroup
	var cur *robotsGroup
	esperandoUA := false // true logo após uma linha User-agent, pra agrupar UAs consecutivos

	for _, raw := range strings.Split(body, "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		campo, valor, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		campo = strings.ToLower(strings.TrimSpace(campo))
		valor = strings.TrimSpace(valor)

		switch campo {
		case "user-agent":
			if cur == nil || !esperandoUA {
				groups = append(groups, robotsGroup{})
				cur = &groups[len(groups)-1]
			}
			cur.uas = append(cur.uas, strings.ToLower(valor))
			esperandoUA = true
		case "disallow":
			if cur != nil {
				cur.disallow = append(cur.disallow, valor)
			}
			esperandoUA = false
		case "allow":
			if cur != nil {
				cur.allow = append(cur.allow, valor)
			}
			esperandoUA = false
		case "crawl-delay":
			if cur != nil {
				if n, err := strconv.ParseFloat(valor, 64); err == nil {
					cur.crawlDelay = time.Duration(n * float64(time.Second))
				}
			}
			esperandoUA = false
		}
	}

	escolhido := escolherGrupo(groups, strings.ToLower(ua))
	if escolhido == nil {
		return &robotsPolicy{}
	}
	return &robotsPolicy{
		disallow:   escolhido.disallow,
		allow:      escolhido.allow,
		crawlDelay: escolhido.crawlDelay,
	}
}

// escolherGrupo prefere um grupo cujo UA casa com o nosso token; cai no "*".
func escolherGrupo(groups []robotsGroup, ua string) *robotsGroup {
	var curinga *robotsGroup
	for i := range groups {
		for _, g := range groups[i].uas {
			if g == "*" {
				curinga = &groups[i]
			} else if strings.Contains(ua, g) {
				return &groups[i]
			}
		}
	}
	return curinga
}

// allowed aplica a regra de maior prefixo: o Allow ou Disallow mais específico
// (mais longo) vence; empate vai pro Allow. Disallow vazio não bloqueia nada.
func (p *robotsPolicy) allowed(path string) bool {
	maisLongoDisallow := -1
	for _, d := range p.disallow {
		if d == "" {
			continue // "Disallow:" vazio = sem restrição
		}
		if prefixoRobots(path, d) && len(d) > maisLongoDisallow {
			maisLongoDisallow = len(d)
		}
	}
	if maisLongoDisallow < 0 {
		return true
	}
	for _, a := range p.allow {
		if a == "" {
			continue
		}
		if prefixoRobots(path, a) && len(a) >= maisLongoDisallow {
			return true
		}
	}
	return false
}

// prefixoRobots casa o padrão do robots contra o caminho. Suporta o curinga
// '*' no fim (prefixo) de forma simples; o resto é comparação de prefixo.
func prefixoRobots(path, pattern string) bool {
	pattern = strings.TrimSuffix(pattern, "*")
	return strings.HasPrefix(path, pattern)
}
