package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/starfederation/datastar-go/datastar"
)

const (
	copyBufferSize    = 32 * 1024
	backendPollDelay  = 100 * time.Millisecond
	readHeaderTimeout = 5 * time.Second
)

func main() {
	publicAddr := envDefault("DEV_PROXY_ADDR", "0.0.0.0:"+envDefault("PORT", "8080"))
	backendURL, err := url.Parse(envDefault("BACKEND_URL", "http://127.0.0.1:18080"))
	if err != nil {
		log.Fatalf("parse backend url: %v", err)
	}

	proxy := newProxy(backendURL)
	mux := http.NewServeMux()
	mux.HandleFunc("/events", proxy.handleEvents)
	mux.HandleFunc("/", proxy.reverseProxy.ServeHTTP)

	server := &http.Server{
		Addr:              publicAddr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	log.Println("dev proxy listening on", "http://"+publicAddr, "->", backendURL)
	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve dev proxy: %v", err)
	}
}

type proxyServer struct {
	backend      *url.URL
	reverseProxy *httputil.ReverseProxy
	client       *http.Client
}

func newProxy(backend *url.URL) *proxyServer {
	rp := httputil.NewSingleHostReverseProxy(backend)
	return &proxyServer{
		backend:      backend,
		reverseProxy: rp,
		client:       &http.Client{Timeout: 0},
	}
}

func (p *proxyServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		if err := p.bridgeEvents(w, r); err != nil && r.Context().Err() == nil {
			log.Printf("events backend disconnected: %v", err)
		}
		if r.Context().Err() != nil {
			return
		}
		if !p.waitForBackend(r.Context()) {
			return
		}
		_ = datastar.NewSSE(w, r).ExecuteScript("window.location.reload()")
	}
}

func (p *proxyServer) bridgeEvents(w http.ResponseWriter, r *http.Request) error {
	u := p.backend.ResolveReference(&url.URL{Path: "/events", RawQuery: r.URL.RawQuery})
	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		u.String(),
		http.NoBody,
	)
	if err != nil {
		return err
	}
	copyRequestHeaders(req.Header, r.Header)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return errors.New(resp.Status)
	}

	_, err = copyFlush(w, resp.Body)
	return err
}

func copyRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyFlush(dst http.ResponseWriter, src io.Reader) (int64, error) {
	buf := make([]byte, copyBufferSize)
	var written int64
	for {
		n, er := src.Read(buf)
		if n > 0 {
			nw, ew := dst.Write(buf[:n])
			written += int64(nw)
			if f, ok := dst.(http.Flusher); ok {
				f.Flush()
			}
			if ew != nil {
				return written, ew
			}
			if nw != n {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			return written, er
		}
	}
}

func (p *proxyServer) waitForBackend(ctx context.Context) bool {
	ticker := time.NewTicker(backendPollDelay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if p.backendHealthy(ctx) {
				return true
			}
		}
	}
}

func (p *proxyServer) backendHealthy(ctx context.Context) bool {
	u := p.backend.ResolveReference(&url.URL{Path: "/healthz"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
