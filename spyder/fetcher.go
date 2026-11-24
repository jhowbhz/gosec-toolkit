package main

import "context"

// requestInfo é uma requisição de rede que a página disparou (XHR/fetch/asset).
// É daqui que saem os endpoints reais de uma SPA.
type requestInfo struct {
	URL          string
	Method       string
	Status       int
	ResourceType string // "xhr", "fetch", "script", "stylesheet", "image", "document", ...
}

// pageResult é o que uma renderização devolve sobre uma URL.
type pageResult struct {
	FinalURL string
	Status   int
	Title    string
	Links    []string      // hrefs encontrados no DOM
	Requests []requestInfo // requisições de rede observadas no load
}

// fetcher renderiza uma URL e devolve o que achou. É a costura que deixa o
// crawler testável sem Chrome: nos testes entra um fake; em produção, chromedp.
type fetcher interface {
	fetch(ctx context.Context, rawurl string, headers []headerKV) (pageResult, error)
}
