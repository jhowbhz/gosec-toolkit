package main

import (
	"sort"
	"strconv"
)

// mudanca é um campo que difere entre duas entradas de mesma chave.
type mudanca struct {
	Key    string
	URL    string
	Method string
	Campo  string // status | kind | sha256
	De     string
	Para   string
}

// resultado é o drift entre dois scans.
type resultado struct {
	Adicionados []entrada
	Removidos   []entrada
	Alterados   []mudanca
}

// diff compara dois scans normalizados. `only` filtra os tipos de mudança
// (added, removed, changed); vazio significa todos. Saída ordenada por chave
// para que duas execuções sobre os mesmos arquivos deem texto idêntico — o
// range sobre map é aleatório e geraria diffs do diff.
func diff(velho, novo map[string]entrada, only map[string]bool) resultado {
	quer := func(tipo string) bool { return len(only) == 0 || only[tipo] }

	var r resultado
	if quer(mudAdicionado) {
		for k, e := range novo {
			if _, existia := velho[k]; !existia {
				r.Adicionados = append(r.Adicionados, e)
			}
		}
	}
	if quer(mudRemovido) {
		for k, e := range velho {
			if _, existe := novo[k]; !existe {
				r.Removidos = append(r.Removidos, e)
			}
		}
	}
	if quer(mudAlterado) {
		for k, antes := range velho {
			depois, ok := novo[k]
			if !ok {
				continue
			}
			r.Alterados = append(r.Alterados, camposAlterados(antes, depois)...)
		}
	}

	sort.Slice(r.Adicionados, func(i, j int) bool { return r.Adicionados[i].Key < r.Adicionados[j].Key })
	sort.Slice(r.Removidos, func(i, j int) bool { return r.Removidos[i].Key < r.Removidos[j].Key })
	sort.Slice(r.Alterados, func(i, j int) bool {
		a, b := r.Alterados[i], r.Alterados[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Campo < b.Campo
	})
	return r
}

// camposAlterados devolve uma mudança por campo que difere. Um campo vazio nos
// dois lados (ex.: kind no jsonl) é igual e não gera ruído.
func camposAlterados(antes, depois entrada) []mudanca {
	base := mudanca{Key: antes.Key, URL: antes.URL, Method: antes.Method}
	var out []mudanca
	adicionar := func(campo, de, para string) {
		if de == para {
			return
		}
		m := base
		m.Campo, m.De, m.Para = campo, de, para
		out = append(out, m)
	}
	adicionar("status", strconv.Itoa(antes.Status), strconv.Itoa(depois.Status))
	adicionar("kind", antes.Kind, depois.Kind)
	adicionar("sha256", antes.Hash, depois.Hash)
	return out
}
