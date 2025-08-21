package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/fdo-server-wrapper/internal/ledger"
)

// FDOProxy represents a reverse proxy that runs the FDO server as a backend
type FDOProxy struct {
	backendURL   *url.URL
	backendCmd   *exec.Cmd
	backendPort  int
	ledgerClient LedgerClient
	middleware   []Middleware
	server       *http.Server
	mu           sync.Mutex
}

// LedgerClient defines the minimal surface the proxy needs from the ledger layer
type LedgerClient interface {
	GetProductItemPassport(ctx context.Context, productUUID string) (*ledger.ProductItemPassport, error)
	CreateCommissioningPassport(ctx context.Context, req *ledger.CommissioningCreateRequest) error
}

// Data models live in the ledger package to avoid duplication

// Middleware interface for request/response processing
type Middleware interface {
	ProcessRequest(ctx context.Context, req *http.Request) error
	ProcessResponse(ctx context.Context, resp *http.Response) error
}

// NewFDOProxy creates a new FDO proxy server
func NewFDOProxy(
	fdoServerPath string,
	fdoArgs []string,
	listenAddr string,
	ledgerClient LedgerClient,
	middleware []Middleware,
) *FDOProxy {
	return &FDOProxy{
		backendPort:  8081, // FDO server will run on this port
		ledgerClient: ledgerClient,
		middleware:   middleware,
	}
}

// Start starts the proxy server and the backend FDO server
func (p *FDOProxy) Start(ctx context.Context, listenAddr string) error {
	// Start the backend FDO server
	if err := p.startBackendServer(ctx); err != nil {
		return fmt.Errorf("failed to start backend FDO server: %w", err)
	}

	// Wait for backend to be ready
	if err := p.waitForBackend(); err != nil {
		return fmt.Errorf("backend server not ready: %w", err)
	}

	// Create reverse proxy
	backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", p.backendPort))
	if err != nil {
		return fmt.Errorf("invalid backend URL: %w", err)
	}
	p.backendURL = backendURL

	// Create proxy handler
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ModifyResponse = p.modifyResponse
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	// Create server with middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := p.processRequest(ctx, r); err != nil {
			slog.Error("Request processing failed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	p.server = &http.Server{
		Addr:    listenAddr,
		Handler: handler,
	}

	slog.Info("FDO proxy server starting", "listen_addr", listenAddr, "backend_port", p.backendPort)
	return p.server.ListenAndServe()
}

// Stop stops the proxy server and the backend FDO server
func (p *FDOProxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop proxy server
	if p.server != nil {
		if err := p.server.Shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown proxy server", "error", err)
		}
	}

	// Stop backend server
	if p.backendCmd != nil && p.backendCmd.Process != nil {
		if err := p.backendCmd.Process.Kill(); err != nil {
			slog.Error("Failed to kill backend process", "error", err)
		}
	}

	return nil
}

// startBackendServer starts the FDO server as a backend process
func (p *FDOProxy) startBackendServer(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Build FDO server command
	args := []string{
		"-db", "./fdo-backend.db",
		"-http", fmt.Sprintf("localhost:%d", p.backendPort),
		"-debug",
	}

	// Create command
	p.backendCmd = exec.CommandContext(ctx, "go", append([]string{"run", "./cmd/server"}, args...)...)
	p.backendCmd.Dir = "../go-fdo" // Path to go-fdo repository
	p.backendCmd.Stdout = os.Stdout
	p.backendCmd.Stderr = os.Stderr

	// Start the backend server
	if err := p.backendCmd.Start(); err != nil {
		return fmt.Errorf("failed to start FDO server: %w", err)
	}

	slog.Info("Backend FDO server started", "pid", p.backendCmd.Process.Pid, "port", p.backendPort)
	return nil
}

// waitForBackend waits for the backend server to be ready
func (p *FDOProxy) waitForBackend() error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for backend server")
		case <-ticker.C:
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", p.backendPort))
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// processRequest processes the request through middleware
func (p *FDOProxy) processRequest(ctx context.Context, req *http.Request) error {
	for _, mw := range p.middleware {
		if err := mw.ProcessRequest(ctx, req); err != nil {
			return fmt.Errorf("middleware request processing failed: %w", err)
		}
	}
	return nil
}

// modifyResponse processes the response through middleware
func (p *FDOProxy) modifyResponse(resp *http.Response) error {
	ctx := context.Background()
	for _, mw := range p.middleware {
		if err := mw.ProcessResponse(ctx, resp); err != nil {
			slog.Error("Middleware response processing failed", "error", err)
			// Don't fail the response, just log the error
		}
	}
	return nil
}
