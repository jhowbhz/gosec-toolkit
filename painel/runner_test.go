package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestLineWriterQuebraLinhas(t *testing.T) {
	var got []string
	w := novoLineWriter(func(l string) { got = append(got, l) })
	w.Write([]byte("ab"))
	w.Write([]byte("c\r\nde"))
	w.Write([]byte("f\ng\n"))
	w.flush()

	quero := []string{"abc", "def", "g"}
	if len(got) != len(quero) {
		t.Fatalf("linhas = %v, quero %v", got, quero)
	}
	for i := range quero {
		if got[i] != quero[i] {
			t.Errorf("linha %d = %q, quero %q", i, got[i], quero[i])
		}
	}
}

func TestRodarProcessoCapturaSaidaEExit(t *testing.T) {
	cmd := helperCommand("linhas")
	dir := t.TempDir()
	logPath := filepath.Join(dir, "output.log")

	var mu sync.Mutex
	var linhas []string
	code, err := rodarProcesso(cmd, logPath, func(l string) {
		mu.Lock()
		linhas = append(linhas, l)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("rodarProcesso: %v", err)
	}
	if code != 3 {
		t.Errorf("exit = %d, quero 3", code)
	}
	set := map[string]bool{}
	for _, l := range linhas {
		set[l] = true
	}
	for _, q := range []string{"linha stdout", "linha stderr"} {
		if !set[q] {
			t.Errorf("faltou capturar %q (capturado: %v)", q, linhas)
		}
	}
	if b, err := os.ReadFile(logPath); err != nil || len(b) == 0 {
		t.Errorf("output.log deveria ter sido gravado: err=%v len=%d", err, len(b))
	}
}

// helperCommand roda o próprio binário de teste como subprocesso controlado.
func helperCommand(modo string) *exec.Cmd {
	c := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", modo)
	c.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return c
}

// TestHelperProcess não é um teste de verdade: é o "programa" que o
// helperCommand executa, selecionado pela env GO_WANT_HELPER_PROCESS.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) > 0 && args[0] == "linhas" {
		fmt.Fprintln(os.Stdout, "linha stdout")
		fmt.Fprintln(os.Stderr, "linha stderr")
		os.Exit(3)
	}
	os.Exit(0)
}
