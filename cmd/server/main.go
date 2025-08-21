package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fdo-server-wrapper/internal/ledger"
	"github.com/fdo-server-wrapper/internal/middleware"
	"github.com/fdo-server-wrapper/internal/proxy"
)

var (
	// Proxy server flags
	listenAddr string
	fdoPath    string

	// Passport service flags
	productPassportBaseURL string
	commissioningCreateURL string
	caCertPath             string
	clientCertPath         string
	clientKeyPath          string
	enableProductPassport  bool
	ownerID                string

	// Debug flag
	debug bool
)

func init() {
	// Proxy server flags
	flag.StringVar(&listenAddr, "listen", "localhost:8080", "Address to listen on")
	flag.StringVar(&fdoPath, "fdo-path", "../go-fdo", "Path to go-fdo repository")

	// Passport service flags
	flag.StringVar(&productPassportBaseURL, "product-base-url", "", "Base URL for product item passport service (e.g., https://cmulk1.cymanii.org:8443)")
	flag.StringVar(&commissioningCreateURL, "commissioning-url", "", "URL for commissioning passport creation (e.g., http://cmulk1.cymanii.org:8000/create-commissioning-passport)")
	flag.StringVar(&caCertPath, "ca-cert", "", "Path to CA cert PEM for product passport mTLS")
	flag.StringVar(&clientCertPath, "client-cert", "", "Path to client cert PEM for product passport mTLS")
	flag.StringVar(&clientKeyPath, "client-key", "", "Path to client key PEM for product passport mTLS")
	flag.BoolVar(&enableProductPassport, "enable-product-passport", false, "Enable product item passport lookup during DI")
	flag.StringVar(&ownerID, "owner-id", "", "Owner ID for commissioning passports")

	// Debug flag
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
}

func main() {
	flag.Parse()

	// Setup logging
	if debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	// Initialize passport client if configured
	var ledgerClient proxy.LedgerClient
	if productPassportBaseURL != "" || commissioningCreateURL != "" {
		c, err := ledger.NewClient(productPassportBaseURL, commissioningCreateURL, caCertPath, clientCertPath, clientKeyPath)
		if err != nil {
			slog.Warn("Passport client init failed", "error", err)
		} else {
			ledgerClient = c
			slog.Info("Passport client initialized", "product_base", productPassportBaseURL, "commissioning_url", commissioningCreateURL)
		}
	} else {
		slog.Warn("Passport client not configured - functionality will be disabled")
	}

	// Create middleware
	var middlewareList []proxy.Middleware

	// Add DI middleware if product passport is enabled
	if enableProductPassport {
		diMiddleware := middleware.NewDIMiddleware(ledgerClient, enableProductPassport)
		middlewareList = append(middlewareList, diMiddleware)
		slog.Info("DI middleware enabled for product passport")
	}

	// Add TO2 middleware if owner ID is provided
	if ownerID != "" {
		to2Middleware := middleware.NewTO2Middleware(ledgerClient, ownerID)
		middlewareList = append(middlewareList, to2Middleware)
		slog.Info("TO2 middleware enabled for commissioning passport", "owner_id", ownerID)
	}

	// Create and start proxy
	proxy := proxy.NewFDOProxy(fdoPath, nil, listenAddr, ledgerClient, middlewareList)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutdown signal received, stopping proxy...")
		cancel()
	}()

	// Start the proxy
	slog.Info("Starting FDO proxy server", "listen_addr", listenAddr)
	if err := proxy.Start(ctx, listenAddr); err != nil {
		slog.Error("Proxy server error", "error", err)
		os.Exit(1)
	}
}
