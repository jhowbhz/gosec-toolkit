package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// report junta, ordena e apresenta os achados. A dedup por (detector+valor)
// evita repetir o mesmo segredo que aparece em vários arquivos; a ordenação
// por severidade põe o que importa no topo; e o exit code permite usar a
// ferramenta como gate em CI.

// rankSeveridade dá a ordem de exibição (menor = mais grave, vem antes).
var rankSeveridade = map[string]int{sevCritico: 0, sevAlto: 1, sevMedio: 2, sevInfo: 3}

// consolidar deduplica por (detector+valor), conta ocorrências, junta os
// locais, preenche o preview (mascarado ou não) e ordena por severidade.
func consolidar(todos []achado, showFull bool) []achado {
	idx := map[string]*achado{}
	var ordem []string
	for _, a := range todos {
		k := a.Detector + "\x00" + a.Valor
		local := fmt.Sprintf("%s:%d", a.Local, a.Linha)
		if ex, ok := idx[k]; ok {
			ex.Ocorrencias++
			if len(ex.Locais) < 10 {
				ex.Locais = append(ex.Locais, local)
			}
			continue
		}
		novo := a
		novo.Ocorrencias = 1
		novo.Locais = []string{local}
		novo.Preview = preview(a.Valor, showFull)
		idx[k] = &novo
		ordem = append(ordem, k)
	}
	out := make([]achado, 0, len(ordem))
	for _, k := range ordem {
		out = append(out, *idx[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankSeveridade[out[i].Severidade] < rankSeveridade[out[j].Severidade]
	})
	return out
}

// exitCode devolve 2 se houver qualquer achado secreto (útil como gate em CI),
// senão 0.
func exitCode(achados []achado) int {
	for _, a := range achados {
		if a.Classe == classeSecreto {
			return 2
		}
	}
	return 0
}

// escreverJSONL grava um achado por linha (o manifesto / evidência).
func escreverJSONL(w io.Writer, achados []achado) {
	for _, a := range achados {
		if b, err := json.Marshal(a); err == nil {
			w.Write(b)
			w.Write([]byte("\n"))
		}
	}
}

// contagem resume quantos achados há por classe.
func contagem(achados []achado) (secretos, publicaveis, suspeitos int) {
	for _, a := range achados {
		switch a.Classe {
		case classeSecreto:
			secretos++
		case classePublicavel:
			publicaveis++
		case classeSuspeito:
			suspeitos++
		}
	}
	return
}

// escreverConsole imprime o relatório agrupado, o mais grave primeiro.
func escreverConsole(w io.Writer, achados []achado) {
	for _, a := range achados {
		local := a.Locais[0]
		if a.Ocorrencias > 1 {
			local = fmt.Sprintf("%s (+%d)", local, a.Ocorrencias-1)
		}
		extra := ""
		if a.Extra != "" {
			extra = "  " + a.Extra
		}
		fmt.Fprintf(w, "[%-11s] %-22s %s  %s%s\n", a.Severidade, a.Detector, a.Preview, local, extra)
	}
	s, p, q := contagem(achados)
	fmt.Fprintf(w, "\nresumo: %d secretos, %d publicáveis, %d suspeitos (%d achados únicos)\n", s, p, q, len(achados))
}

// textoResumo monta o conteúdo do arquivo _CREDENCIAIS-ENCONTRADAS.txt.
func textoResumo(achados []achado) string {
	var b strings.Builder
	b.WriteString("Credenciais/segredos encontrados pelo spyder-credentials.\n")
	b.WriteString("Valores mascarados por padrão. 'secreto' = nunca deveria vazar; ")
	b.WriteString("'publicavel' = público por desenho; 'suspeito' = conferir à mão.\n\n")
	for _, a := range achados {
		fmt.Fprintf(&b, "[%s] %s — %s\n", strings.ToUpper(a.Severidade), a.Detector, a.Classe)
		fmt.Fprintf(&b, "      %s", a.Preview)
		if a.Extra != "" {
			fmt.Fprintf(&b, "   (%s)", a.Extra)
		}
		b.WriteString("\n      " + strings.Join(a.Locais, ", "))
		if a.Ocorrencias > len(a.Locais) {
			fmt.Fprintf(&b, " e mais %d", a.Ocorrencias-len(a.Locais))
		}
		b.WriteString("\n\n")
	}
	return b.String()
}
