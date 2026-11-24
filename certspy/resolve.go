package main

import (
	"context"
	"net"
	"sort"
	"sync"
	"time"
)

// resolverTodos preenche Vivo e IPs de cada domínio. "Vivo" significa só que
// o nome tem A/AAAA: o CT lista histórico, e nome sem registro é certificado
// antigo, não superfície no ar. Erro de DNS vira não-vivo, não falha da
// execução. O pool limita a pressão sobre o resolvedor.
func resolverTodos(ctx context.Context, doms []dominio, concorrencia int, timeout time.Duration, aviso func(dominio)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, concorrencia)
	for i := range doms {
		sem <- struct{}{}
		wg.Add(1)
		go func(d *dominio) {
			defer func() { <-sem; wg.Done() }()
			d.IPs, d.Vivo = resolverNome(ctx, d.Nome, timeout)
			if aviso != nil {
				aviso(*d)
			}
		}(&doms[i])
	}
	wg.Wait()
}

func resolverNome(ctx context.Context, nome string, timeout time.Duration) ([]string, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupHost(ctx, nome)
	if err != nil || len(ips) == 0 {
		return nil, false
	}
	sort.Strings(ips)
	return ips, true
}
