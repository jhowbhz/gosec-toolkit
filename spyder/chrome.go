package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// robotsUA é o token com que nos identificamos no robots.txt.
const robotsUA = "spyder"

// chromeFetcher renderiza páginas em Chrome headless (chromedp). Mantém um
// browser só; cada fetch abre uma aba nova. Captura os links do DOM e as
// requisições de rede (XHR/fetch/assets) que a página dispara no load.
type chromeFetcher struct {
	base   context.Context
	settle time.Duration
}

// newChromeFetcher sobe o browser. O cleanup devolvido encerra tudo.
func newChromeFetcher(chromePath string, settle time.Duration) (*chromeFetcher, func(), error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.Flag("disable-gpu", true),
	)
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	// Silencia os "could not unmarshal event" do cdproto fixado (campos novos
	// de CDP que a versão antiga não conhece) — são inofensivos e poluem.
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...interface{}) {}))
	if err := chromedp.Run(browserCtx); err != nil { // sobe o browser
		cancelBrowser()
		cancelAlloc()
		return nil, nil, err
	}
	cleanup := func() { cancelBrowser(); cancelAlloc() }
	return &chromeFetcher{base: browserCtx, settle: settle}, cleanup, nil
}

func (f *chromeFetcher) fetch(ctx context.Context, rawurl string, headers []headerKV) (pageResult, error) {
	tabCtx, cancelTab := chromedp.NewContext(f.base)
	defer cancelTab()
	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancelTimeout()

	// Captura as requisições de rede, correlacionando request e response.
	var mu sync.Mutex
	reqs := make(map[network.RequestID]*requestInfo)
	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			mu.Lock()
			reqs[e.RequestID] = &requestInfo{
				URL:          e.Request.URL,
				Method:       e.Request.Method,
				ResourceType: strings.ToLower(e.Type.String()),
			}
			mu.Unlock()
		case *network.EventResponseReceived:
			mu.Lock()
			if ri, ok := reqs[e.RequestID]; ok {
				ri.Status = int(e.Response.Status)
			}
			mu.Unlock()
		}
	})

	var title, finalURL string
	var hrefs []string
	tasks := chromedp.Tasks{
		network.Enable(),
		network.SetExtraHTTPHeaders(headersDeRede(headers)),
		chromedp.Navigate(rawurl),
		chromedp.Sleep(f.settle), // deixa os XHRs do load dispararem
		chromedp.Location(&finalURL),
		chromedp.Title(&title),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href]')).map(a => a.getAttribute('href'))`, &hrefs),
	}
	if err := chromedp.Run(tabCtx, tasks); err != nil {
		return pageResult{FinalURL: rawurl}, err
	}

	mu.Lock()
	defer mu.Unlock()
	var out []requestInfo
	status := 0
	for _, ri := range reqs {
		out = append(out, *ri)
		if ri.ResourceType == "document" && status == 0 {
			status = ri.Status // status da navegação principal
		}
	}
	return pageResult{
		FinalURL: finalURL,
		Status:   status,
		Title:    title,
		Links:    hrefs,
		Requests: out,
	}, nil
}

func headersDeRede(headers []headerKV) network.Headers {
	h := network.Headers{}
	for _, kv := range headers {
		h[kv.Name] = kv.Value
	}
	return h
}
