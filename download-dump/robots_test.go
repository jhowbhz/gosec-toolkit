package main

import (
	"testing"
	"time"
)

func TestRobotsGrupoCuringa(t *testing.T) {
	body := []byte(`
User-agent: *
Disallow: /private
Crawl-delay: 5
`)
	r := parseRobots(body, "download-dump/1.0")
	if r.allowed("/private/x") {
		t.Error("/private/x deveria ser bloqueado")
	}
	if !r.allowed("/ok") {
		t.Error("/ok deveria ser permitido")
	}
	if r.crawlDelay() != 5*time.Second {
		t.Errorf("crawlDelay = %v, quero 5s", r.crawlDelay())
	}
}

func TestRobotsGrupoEspecificoVenceCuringa(t *testing.T) {
	body := []byte(`
User-agent: *
Disallow: /

User-agent: download-dump
Disallow: /tmp
Allow: /
`)
	r := parseRobots(body, "download-dump/1.0")
	// O grupo específico (download-dump) é o único que vale; o Disallow: /
	// do curinga não se aplica a nós.
	if !r.allowed("/page") {
		t.Error("/page deveria ser permitido pelo grupo específico")
	}
	// Disallow /tmp (mais longo) vence Allow / (mais curto).
	if r.allowed("/tmp/x") {
		t.Error("/tmp/x deveria ser bloqueado")
	}
}

func TestRobotsCuringaEAncora(t *testing.T) {
	body := []byte(`
User-agent: *
Disallow: /*.php$
`)
	r := parseRobots(body, "download-dump/1.0")
	if r.allowed("/index.php") {
		t.Error("/index.php deveria ser bloqueado")
	}
	if r.allowed("/a/b.php") {
		t.Error("/a/b.php deveria ser bloqueado")
	}
	if !r.allowed("/index.phpx") {
		t.Error("/index.phpx deveria ser permitido ($ ancora o fim)")
	}
}

func TestRobotsDisallowVazioPermiteTudo(t *testing.T) {
	body := []byte(`
User-agent: *
Disallow:
`)
	r := parseRobots(body, "download-dump/1.0")
	if !r.allowed("/qualquer/coisa") {
		t.Error("Disallow vazio deveria permitir tudo")
	}
}

func TestRobotsVazioPermiteTudo(t *testing.T) {
	r := parseRobots(nil, "download-dump/1.0")
	if !r.allowed("/qualquer") {
		t.Error("robots vazio deveria permitir tudo")
	}
	if r.crawlDelay() != 0 {
		t.Errorf("crawlDelay sem regra = %v, quero 0", r.crawlDelay())
	}
}
