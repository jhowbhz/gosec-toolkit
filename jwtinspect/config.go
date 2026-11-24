package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA da entrada; ler arquivo de tokens/segredos é I/O e fica
// para o runtime. Também não exige token aqui: ele pode vir do stdin.
type Config struct {
	Tokens      []string // --token, repetível
	TokenFile   string   // arquivo com um token por linha
	Secrets     []string // --secret, repetível: candidatos de segredo HMAC
	SecretsFile string   // wordlist de candidatos, um por linha
	LeewaySec   int      // folga de relógio para exp/nbf/iat, em segundos
	MaxLifeH    int      // acima disso (exp - iat) é "vida longa"; 0 desliga
	LogFile     string   // caminho do JSONL; vazio = automático
	Quiet       bool     // não imprime o resultado por token no console

	// o flag package não distingue "--token-file=" de flag ausente; sem isto
	// validate não saberia que o usuário passou um caminho vazio.
	tokenFileDado   bool
	secretsFileDado bool
}

const (
	defaultLeewaySec = 60
	defaultMaxLifeH  = 24
)

// listaFlag coleta uma flag repetível num slice de strings.
type listaFlag []string

func (l *listaFlag) String() string { return strings.Join(*l, ", ") }
func (l *listaFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("jwtinspect", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var tokens, secrets listaFlag
	fs.Var(&tokens, "token", "JWT a inspecionar (repetível)")
	fs.StringVar(&c.TokenFile, "token-file", "", "arquivo com um token por linha")
	fs.Var(&secrets, "secret", "candidato de segredo HMAC a testar offline (repetível)")
	fs.StringVar(&c.SecretsFile, "secrets", "", "wordlist de candidatos de segredo, um por linha")
	fs.IntVar(&c.LeewaySec, "leeway", defaultLeewaySec, "folga de relógio em segundos para exp/nbf/iat")
	fs.IntVar(&c.MaxLifeH, "max-life", defaultMaxLifeH, "vida máxima em horas antes de sinalizar vida longa; 0 desliga")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do JSONL; vazio = jwtinspect-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o resultado por token no console")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "token-file":
			c.tokenFileDado = true
		case "secrets":
			c.secretsFileDado = true
		}
	})
	c.Tokens = tokens
	c.Secrets = secrets
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere só a forma: números não-negativos e caminhos não vazios.
// Se o arquivo existe ou se há token em algum lugar é problema do runtime.
func (c Config) validate() error {
	if c.LeewaySec < 0 {
		return fmt.Errorf("--leeway não pode ser negativo: %d", c.LeewaySec)
	}
	if c.MaxLifeH < 0 {
		return fmt.Errorf("--max-life não pode ser negativo: %d", c.MaxLifeH)
	}
	if c.tokenFileDado && strings.TrimSpace(c.TokenFile) == "" {
		return errors.New("--token-file sem caminho")
	}
	if c.secretsFileDado && strings.TrimSpace(c.SecretsFile) == "" {
		return errors.New("--secrets sem caminho")
	}
	return nil
}
