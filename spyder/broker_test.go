package main

import (
	"testing"
	"time"
)

func TestBrokerEntregaAoAssinante(t *testing.T) {
	b := newBroker()
	ch, cancelar := b.subscribe()
	defer cancelar()

	b.publish(route{URL: "https://x/a"})

	select {
	case r := <-ch:
		if r.URL != "https://x/a" {
			t.Errorf("recebeu %q, quero https://x/a", r.URL)
		}
	case <-time.After(time.Second):
		t.Fatal("assinante não recebeu o evento")
	}
}

func TestBrokerCancelarParaDeReceber(t *testing.T) {
	b := newBroker()
	ch, cancelar := b.subscribe()
	cancelar()

	// publish depois do cancel não pode entregar nem dar panic (canal fechado)
	b.publish(route{URL: "https://x/b"})

	if _, aberto := <-ch; aberto {
		t.Error("canal deveria estar fechado após cancelar")
	}
}

func TestBrokerNaoBloqueiaComAssinanteLento(t *testing.T) {
	b := newBroker()
	_, cancelar := b.subscribe() // nunca lê
	defer cancelar()

	feito := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.publish(route{URL: "https://x"})
		}
		close(feito)
	}()
	select {
	case <-feito:
	case <-time.After(2 * time.Second):
		t.Fatal("publish travou com assinante lento (deveria descartar)")
	}
}
