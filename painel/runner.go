package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// runner executa as ferramentas e distribui a saída ao vivo. Cada linha de
// stdout/stderr é gravada no output.log do run E publicada para os inscritos
// (as conexões SSE do navegador). Linhas já emitidas ficam num histórico para
// que um inscrito que chega atrasado receba tudo desde o começo.

// lineWriter acumula bytes e chama sink uma vez por linha completa. Protegido
// por mutex porque stdout e stderr escrevem nele de goroutines diferentes.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	sink func(string)
}

func novoLineWriter(sink func(string)) *lineWriter {
	return &lineWriter{sink: sink}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		linha := string(bytes.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
		w.sink(linha)
	}
	return len(p), nil
}

// flush emite o que sobrou no buffer sem o '\n' final (última linha).
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.sink(string(bytes.TrimRight(w.buf, "\r")))
		w.buf = nil
	}
}

// rodarProcesso executa o comando, grava a saída no logPath e chama emit por
// linha. Devolve o exit code; saída com código ≠ 0 não é erro de execução.
func rodarProcesso(cmd *exec.Cmd, logPath string, emit func(string)) (int, error) {
	lf, err := os.Create(logPath)
	if err != nil {
		return -1, err
	}
	defer lf.Close()

	w := novoLineWriter(func(l string) {
		emit(l)
		lf.WriteString(l + "\n")
	})
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err = cmd.Wait()
	w.flush()

	if cmd.ProcessState != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return cmd.ProcessState.ExitCode(), nil
		}
		if err == nil {
			return cmd.ProcessState.ExitCode(), nil
		}
	}
	return -1, err
}

// --- pub/sub de streaming ao vivo ---

// fluxo guarda os inscritos e o histórico de linhas de um run em andamento.
type fluxo struct {
	mu        sync.Mutex
	hist      []string
	inscritos map[chan string]bool
	fechado   bool
}

// runner orquestra execuções e seus fluxos de saída.
type runner struct {
	store    *store
	resolver func(f ferramenta) (string, []string, error) // resolve binário + prefixo de argv
	mu       sync.Mutex
	fluxos   map[string]*fluxo
	cmds     map[string]*exec.Cmd // processos vivos, para poder parar
}

func novoRunner(st *store, resolver func(ferramenta) (string, []string, error)) *runner {
	return &runner{store: st, resolver: resolver, fluxos: map[string]*fluxo{}, cmds: map[string]*exec.Cmd{}}
}

// iniciar cria o run, resolve o binário, injeta as flags de saída apontando
// para a pasta do run e executa em background, publicando a saída ao vivo.
func (rn *runner) iniciar(f ferramenta, valores map[string][]string) (run, error) {
	argv, err := montarArgv(f, valores)
	if err != nil {
		return run{}, err
	}
	r, err := rn.store.criarRun(f.ID, valores, argv)
	if err != nil {
		return run{}, err
	}
	bin, prefixo, err := rn.resolver(f)
	if err != nil {
		rn.finalizar(r, -1, err)
		return r, err
	}

	dir := rn.store.dirDoRun(r.ID)
	final := append([]string{}, prefixo...)
	final = append(final, argv...)
	final = append(final, flagsDeSaida(f, dir)...)

	cmd := exec.Command(bin, final...)
	cmd.Dir = dir
	fx := &fluxo{inscritos: map[chan string]bool{}}
	rn.mu.Lock()
	rn.fluxos[r.ID] = fx
	rn.cmds[r.ID] = cmd
	rn.mu.Unlock()

	go func() {
		code, execErr := rodarProcesso(cmd, filepath.Join(dir, "output.log"), func(l string) {
			fx.publicar(l)
		})
		fx.encerrar()
		rn.mu.Lock()
		delete(rn.cmds, r.ID)
		rn.mu.Unlock()
		rn.finalizar(r, code, execErr)
	}()
	return r, nil
}

// parar mata o processo de um run em andamento. Devolve false se já terminou.
func (rn *runner) parar(id string) bool {
	rn.mu.Lock()
	cmd := rn.cmds[id]
	rn.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return cmd.Process.Kill() == nil
}

// flagsDeSaida injeta as flags de saída da ferramenta apontando para a pasta
// do run (o usuário nunca controla onde grava).
func flagsDeSaida(f ferramenta, dir string) []string {
	var out []string
	if f.SaidaDir != "" {
		out = append(out, "--"+f.SaidaDir+"="+dir)
	}
	if f.SaidaLog != "" {
		out = append(out, "--"+f.SaidaLog+"="+dir+string(os.PathSeparator)+f.ID+".jsonl")
	}
	return out
}

// finalizar grava o status final do run.
func (rn *runner) finalizar(r run, code int, execErr error) {
	agora := time.Now()
	r.Fim = &agora
	r.Exit = &code
	if execErr != nil && code < 0 {
		r.Status = "erro"
	} else if code == 0 {
		r.Status = "ok"
	} else {
		r.Status = "saiu-" + itoa(code)
	}
	rn.store.salvarMeta(r)
}

// inscrever registra um canal que recebe primeiro o histórico e depois as
// novas linhas; o segundo retorno diz se o fluxo já terminou.
func (rn *runner) inscrever(id string) (chan string, func(), bool) {
	rn.mu.Lock()
	fx := rn.fluxos[id]
	rn.mu.Unlock()
	if fx == nil {
		return nil, func() {}, true
	}
	return fx.inscrever()
}

func (fx *fluxo) publicar(l string) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	fx.hist = append(fx.hist, l)
	for ch := range fx.inscritos {
		select {
		case ch <- l:
		default:
		}
	}
}

func (fx *fluxo) encerrar() {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	fx.fechado = true
	for ch := range fx.inscritos {
		close(ch)
		delete(fx.inscritos, ch)
	}
}

func (fx *fluxo) inscrever() (chan string, func(), bool) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	ch := make(chan string, len(fx.hist)+64)
	for _, l := range fx.hist {
		ch <- l
	}
	if fx.fechado {
		close(ch)
		return ch, func() {}, true
	}
	fx.inscritos[ch] = true
	cancelar := func() {
		fx.mu.Lock()
		defer fx.mu.Unlock()
		if fx.inscritos[ch] {
			delete(fx.inscritos, ch)
			close(ch)
		}
	}
	return ch, cancelar, false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
