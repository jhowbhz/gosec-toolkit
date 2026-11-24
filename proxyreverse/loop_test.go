package main

import "testing"

func TestChecarLoopDiretoPegaLoopbackNaMesmaPorta(t *testing.T) {
	// O caso que derrubou o usuário: --from no hosts -> 127.0.0.1, escuta :80,
	// e --to=http://127.0.0.1 (porta 80) = o próprio proxy.
	cfg, err := parseConfig([]string{"--from=meusite.com.br", "--to=http://127.0.0.1"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if err := checarLoopDireto(cfg); err == nil {
		t.Fatal("quero erro: --to=http://127.0.0.1 na porta 80 que o proxy escuta é loop")
	}
}

func TestChecarLoopDiretoPortaDiferenteNaoEhLoop(t *testing.T) {
	// Caso legítimo de dev: encaminhar pra um servidor local em OUTRA porta.
	cfg, err := parseConfig([]string{"--from=site.local", "--to=http://127.0.0.1:3000", "--http=127.0.0.1:8080"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if err := checarLoopDireto(cfg); err != nil {
		t.Errorf("porta diferente não é loop: %v", err)
	}
}

func TestChecarLoopDiretoDestinoRealNaoEhLoop(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=meusite.com.br", "--to=https://203.0.113.5"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if err := checarLoopDireto(cfg); err != nil {
		t.Errorf("destino real não-loopback não é loop: %v", err)
	}
}

func TestChecarLoopDiretoLocalhost(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local", "--to=http://localhost", "--http=:80"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if err := checarLoopDireto(cfg); err == nil {
		t.Fatal("quero erro: localhost:80 é a porta que o proxy escuta")
	}
}
