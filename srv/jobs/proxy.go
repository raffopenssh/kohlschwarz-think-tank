package jobs

// Minimal free-proxy pool, borrowed in spirit from srtm-lidar-at/bev_proxy.py.
// Only used for the handful of Austrian portals that geo-block cloud IPs.
// Candidates come from a continuously validated public list; we probe them
// against the target itself and keep the ones that answer, with cooldowns.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var proxyLists = []string{
	"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/http/global/http_checked.txt",
	"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/all.txt",
	"https://raw.githubusercontent.com/hendrikbgr/Free-Proxy-Repo/master/proxy_list.txt",
}

var insecureClient = &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

type proxyPool struct {
	mu      sync.Mutex
	good    []string             // proxies that worked recently, tried first
	bad     map[string]time.Time // cooldown until
	fetched time.Time
	cands   []string
}

var proxies = &proxyPool{bad: map[string]time.Time{}}

func (p *proxyPool) candidates(ctx context.Context) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.fetched) < 30*time.Minute && len(p.cands) > 0 {
		return p.cands
	}
	var all []string
	seen := map[string]bool{}
	for _, u := range proxyLists {
		b, err := get(ctx, u)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			l = strings.TrimPrefix(l, "http://")
			if l == "" || !strings.Contains(l, ":") || seen[l] {
				continue
			}
			seen[l] = true
			all = append(all, l)
		}
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	p.cands, p.fetched = all, time.Now()
	return all
}

// getViaProxy fetches u through free proxies in parallel until ok(body) holds.
func getViaProxy(ctx context.Context, u string, ok func([]byte) bool) ([]byte, error) {
	p := proxies
	p.mu.Lock()
	order := append([]string{}, p.good...)
	p.mu.Unlock()
	for _, c := range p.candidates(ctx) {
		p.mu.Lock()
		until, cooled := p.bad[c]
		p.mu.Unlock()
		if cooled && time.Now().Before(until) {
			continue
		}
		order = append(order, c)
		if len(order) >= 150 {
			break
		}
	}
	if len(order) == 0 {
		return nil, errors.New("no proxy candidates")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	type res struct {
		proxy string
		body  []byte
	}
	results := make(chan res, 1)
	sem := make(chan struct{}, 25)
	var wg sync.WaitGroup
	for _, c := range order {
		select {
		case <-ctx.Done():
		case sem <- struct{}{}:
			wg.Add(1)
			go func(c string) {
				defer wg.Done()
				defer func() { <-sem }()
				b, err := getThrough(ctx, c, u)
				if err == nil && ok(b) {
					select {
					case results <- res{c, b}:
					default:
					}
					return
				}
				p.mu.Lock()
				p.bad[c] = time.Now().Add(6 * time.Hour)
				p.mu.Unlock()
			}(c)
		}
		select {
		case r := <-results:
			p.mu.Lock()
			p.good = append([]string{r.proxy}, p.good...)
			if len(p.good) > 10 {
				p.good = p.good[:10]
			}
			p.mu.Unlock()
			cancel()
			wg.Wait()
			return r.body, nil
		default:
		}
	}
	go func() { wg.Wait(); close(results) }()
	if r, okr := <-results; okr {
		p.mu.Lock()
		p.good = append([]string{r.proxy}, p.good...)
		p.mu.Unlock()
		return r.body, nil
	}
	return nil, fmt.Errorf("no working proxy among %d", len(order))
}

func getThrough(ctx context.Context, proxy, u string) ([]byte, error) {
	pu, err := url.Parse("http://" + proxy)
	if err != nil {
		return nil, err
	}
	cl := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true}}
	return getWith(ctx, cl, u)
}

// getWith is get() with a caller-supplied client (no retry loop).
func getWith(ctx context.Context, cl *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "de-AT,de;q=0.9,en;q=0.8")
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 6<<20))
}
