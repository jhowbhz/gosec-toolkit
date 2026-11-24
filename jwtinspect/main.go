package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "jwtinspect:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, carrega tokens e candidatos,
// audita cada token e imprime o resumo. stdin entra por parâmetro para o
// fluxo "tokens pelo pipe" ser testável.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	tokens, err := carregarTokens(cfg, stdin)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return errors.New("nenhum token para inspecionar (use --token, --token-file ou stdin)")
	}
	segredos, err := carregarSegredos(cfg)
	if err != nil {
		return err
	}

	logPath, logFile, err := abrirRelatorio(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	fmt.Fprintf(stdout, "jwtinspect: %d token(s), %d candidato(s) de segredo\n  relatório: %s\n\n",
		len(tokens), len(segredos), logPath)

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	agora := time.Now()
	for _, raw := range tokens {
		rec.record(inspecionar(raw, cfg, segredos, agora))
	}
	resumoFinal(stdout, rec)
	return nil
}

// inspecionar audita um token e devolve o registro pronto. Erro de decode vira
// registro com Error — um token ruim no meio do arquivo não pode esconder os outros.
func inspecionar(raw string, cfg Config, segredos []string, agora time.Time) registro {
	reg := registro{
		Timestamp: agora.UTC().Format(time.RFC3339),
		TokenID:   idDoToken(raw),
		Issues:    []issue{},
	}
	t, err := decodificar(raw)
	if err != nil {
		reg.Error = err.Error()
		return reg
	}

	leeway := time.Duration(cfg.LeewaySec) * time.Second
	maxLife := time.Duration(cfg.MaxLifeH) * time.Hour

	reg.Alg = t.alg()
	reg.Kid = t.kid()
	reg.Iss, _ = t.Payload["iss"].(string)
	reg.Sub, _ = t.Payload["sub"].(string)
	reg.Aud = t.Payload["aud"]
	reg.Exp = formatarClaim(t.Payload, "exp")
	reg.Iat = formatarClaim(t.Payload, "iat")
	reg.Nbf = formatarClaim(t.Payload, "nbf")
	reg.Validade = situacao(t.Payload, agora, leeway)
	reg.Issues = append(reg.Issues, avaliarClaims(reg.Alg, t.Payload, agora, leeway, maxLife)...)

	if achou, segredo := testarSegredos(t, segredos); achou {
		reg.WeakSecret = segredo
	}
	return reg
}

func formatarClaim(payload map[string]any, nome string) string {
	if t, ok := claimTempo(payload, nome); ok {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}

// idDoToken identifica o token no relatório sem reproduzi-lo (ver registro).
func idDoToken(raw string) string {
	soma := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(soma[:])[:12]
}

// carregarTokens junta --token, --token-file e, se nenhum dos dois veio, o stdin.
func carregarTokens(cfg Config, stdin io.Reader) ([]string, error) {
	tokens := append([]string{}, cfg.Tokens...)
	if cfg.TokenFile != "" {
		linhas, err := lerLinhas(cfg.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("não consegui ler --token-file: %w", err)
		}
		tokens = append(tokens, linhas...)
	}
	if len(tokens) == 0 && cfg.TokenFile == "" {
		linhas, err := linhasDe(stdin)
		if err != nil {
			return nil, fmt.Errorf("não consegui ler o stdin: %w", err)
		}
		tokens = linhas
	}
	return tokens, nil
}

// carregarSegredos junta --secret e a wordlist --secrets.
func carregarSegredos(cfg Config) ([]string, error) {
	segredos := append([]string{}, cfg.Secrets...)
	if cfg.SecretsFile != "" {
		linhas, err := lerLinhas(cfg.SecretsFile)
		if err != nil {
			return nil, fmt.Errorf("não consegui ler --secrets: %w", err)
		}
		segredos = append(segredos, linhas...)
	}
	return segredos, nil
}

func lerLinhas(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return linhasDe(f)
}

// linhasDe devolve as linhas não vazias, sem o fim de linha. Não aplica
// TrimSpace: espaço no começo/fim pode fazer parte de um segredo. O buffer
// grande evita o limite de 64 KB do Scanner — JWTs com muitos claims passam disso.
func linhasDe(r io.Reader) ([]string, error) {
	var linhas []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if l := strings.TrimRight(sc.Text(), "\r\n"); l != "" {
			linhas = append(linhas, l)
		}
	}
	return linhas, sc.Err()
}

// abrirRelatorio cria o arquivo JSONL (caminho pedido ou automático).
func abrirRelatorio(cfg Config) (string, *os.File, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("jwtinspect-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("não consegui criar o relatório %s: %w", path, err)
	}
	return path, f, nil
}

func resumoFinal(w io.Writer, rec *recorder) {
	total, erros, fracos, sev := rec.resumo()
	fmt.Fprintf(w, "\nconcluído: %d token(s), %d erro(s) de decode, %d segredo(s) fraco(s)\n", total, erros, fracos)
	fmt.Fprintf(w, "  achados: %d critical, %d medium, %d low, %d info\n",
		sev[sevCritical]+fracos, sev[sevMedium], sev[sevLow], sev[sevInfo])
}

const uso = `jwtinspect - decodifica e audita JWTs que você fornece (offline, nada vai à rede)

uso:
  jwtinspect --token=<jwt> [opções]
  cat tokens.txt | jwtinspect [opções]

exemplo:
  jwtinspect --token-file=sessoes.txt --secrets=senhas-comuns.txt

entrada de tokens (se nenhuma, lê o stdin, um por linha):
  --token=JWT        token a inspecionar, repetível
  --token-file=PATH  arquivo com um token por linha

opções:
  --secret=STR       candidato de segredo HMAC a testar offline, repetível
  --secrets=PATH     wordlist de candidatos de segredo, um por linha
  --leeway=S         folga de relógio em segundos para exp/nbf/iat (padrão: 60)
  --max-life=H       acima disso (exp - iat) é "vida longa"; 0 desliga (padrão: 24)
  --log-file=PATH    relatório JSONL (padrão: jwtinspect-<ts>.jsonl)
  --quiet            não imprime o resultado por token no console
`
