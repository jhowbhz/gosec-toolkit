package main

import "sync"

// broker distribui as rotas recém-descobertas pros assinantes do dashboard
// (cada conexão SSE é um assinante). Entrega é best-effort: se um assinante
// está lento e seu buffer encheu, o evento é descartado pra não travar o
// crawler — o dashboard recarrega o estado completo do SQLite quando precisa.
type broker struct {
	mu   sync.Mutex
	subs map[chan route]struct{}
}

func newBroker() *broker {
	return &broker{subs: make(map[chan route]struct{})}
}

// subscribe registra um assinante e devolve o canal e a função de cancelar.
func (b *broker) subscribe() (<-chan route, func()) {
	ch := make(chan route, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancelar := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancelar
}

// publish envia a rota a todos os assinantes, sem bloquear.
func (b *broker) publish(r route) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- r:
		default: // assinante lento: descarta
		}
	}
}
