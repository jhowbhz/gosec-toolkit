package main

import (
	"testing"
	"time"
)

func TestRobotsDisallowBloqueiaPrefixo(t *testing.T) {
	p := parseRobots("User-agent: *\nDisallow: /admin\n", "spyder")
	if p.allowed("/admin/users") {
		t.Error("/admin/users deveria ser bloqueado por Disallow: /admin")
	}
	if !p.allowed("/publico") {
		t.Error("/publico não foi bloqueado, deveria ser permitido")
	}
}

func TestRobotsAllowSobrepoeDisallow(t *testing.T) {
	p := parseRobots("User-agent: *\nDisallow: /a\nAllow: /a/ok\n", "spyder")
	if !p.allowed("/a/ok") {
		t.Error("/a/ok deveria ser permitido (Allow mais específico)")
	}
	if p.allowed("/a/segredo") {
		t.Error("/a/segredo deveria continuar bloqueado")
	}
}

func TestRobotsGrupoEspecificoVenceCurtinga(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n\nUser-agent: spyder\nDisallow: /privado\n"
	p := parseRobots(body, "spyder")
	// o grupo do "spyder" manda: só /privado é bloqueado, o resto libera
	if !p.allowed("/qualquer") {
		t.Error("grupo específico do spyder deveria liberar /qualquer")
	}
	if p.allowed("/privado/x") {
		t.Error("/privado deveria ser bloqueado pelo grupo do spyder")
	}
}

func TestRobotsCrawlDelay(t *testing.T) {
	p := parseRobots("User-agent: *\nCrawl-delay: 2\n", "spyder")
	if p.crawlDelay != 2*time.Second {
		t.Errorf("crawlDelay = %v, quero 2s", p.crawlDelay)
	}
}

func TestRobotsSemRegrasLiberaTudo(t *testing.T) {
	p := parseRobots("", "spyder")
	if !p.allowed("/qualquer/coisa") {
		t.Error("sem regras, tudo deveria ser permitido")
	}
}
