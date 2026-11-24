package main

import (
	"strings"
	"testing"
)

func TestEhSensivelPega(t *testing.T) {
	sens := []string{
		"https://site.com/.env",
		"https://site.com/.env.production",
		"https://site.com/.git/config",
		"https://site.com/backup/db.sql",
		"https://site.com/dump.sql.gz",
		"https://site.com/id_rsa",
		"https://site.com/private.key",
		"https://site.com/site-backup.zip",
		"https://site.com/wp-config.php",
		"https://site.com/app.log",
	}
	for _, s := range sens {
		ok, motivo := ehSensivel(mustURL(t, s))
		if !ok {
			t.Errorf("%s deveria ser sensível", s)
		}
		if strings.TrimSpace(motivo) == "" {
			t.Errorf("%s: motivo não pode ser vazio", s)
		}
	}
}

func TestEhSensivelIgnoraNormais(t *testing.T) {
	normais := []string{
		"https://site.com/index.html",
		"https://site.com/css/style.css",
		"https://site.com/js/app.js",
		"https://site.com/img/logo.png",
		"https://site.com/sobre",
	}
	for _, n := range normais {
		if ok, _ := ehSensivel(mustURL(t, n)); ok {
			t.Errorf("%s NÃO deveria ser sensível", n)
		}
	}
}

func TestCaminhosSensiveisComunsTemClassicos(t *testing.T) {
	lista := caminhosSensiveisComuns()
	if len(lista) == 0 {
		t.Fatal("lista de sondagem não pode ser vazia")
	}
	set := map[string]bool{}
	for _, p := range lista {
		set[p] = true
	}
	for _, q := range []string{".env", ".git/config"} {
		if !set[q] {
			t.Errorf("lista de sondagem deveria conter %q", q)
		}
	}
}
