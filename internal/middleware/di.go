package middleware

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fdo-server-wrapper/internal/proxy"
)

// DIMiddleware intercepts DI protocol messages to integrate with passport services.
// It extracts product information during device initialization and logs passport data.
type DIMiddleware struct {
	ledgerClient          proxy.LedgerClient
	enableProductPassport bool
}

// NewDIMiddleware creates middleware for DI protocol integration.
// When enabled, it will attempt to fetch product item passports during DI.AppStart.
func NewDIMiddleware(ledgerClient proxy.LedgerClient, enableProductPassport bool) *DIMiddleware {
	return &DIMiddleware{
		ledgerClient:          ledgerClient,
		enableProductPassport: enableProductPassport,
	}
}

// ProcessRequest handles incoming DI protocol requests.
//
// Contract:
//
//	Preconditions:
//	  - req is not nil and contains valid HTTP request
//	  - ctx is not nil
//
//	Postconditions:
//	  - Returns nil if request is not DI-related or processing succeeds
//	  - Returns error if request processing fails (does not interrupt FDO flow)
//
//	Integration Points:
//	  - DI.AppStart (msg type 10): extracts product UUID and fetches passport
func (m *DIMiddleware) ProcessRequest(ctx context.Context, req *http.Request) error {
	// Only process DI protocol requests
	if !m.isDIRequest(req) {
		return nil
	}

	// Parse message type from URL path: /fdo/101/msg/{msgType}
	pathParts := strings.Split(req.URL.Path, "/")
	if len(pathParts) < 5 {
		return nil
	}

	msgType := pathParts[4]
	if msgType == "10" { // DI.AppStart message
		return m.handleDIAppStart(ctx, req)
	}

	return nil
}

// ProcessResponse handles outgoing DI protocol responses.
//
// Contract:
//
//	Preconditions:
//	  - resp is not nil and contains valid HTTP response
//	  - ctx is not nil
//
//	Postconditions:
//	  - Returns nil if response is not DI-related or processing succeeds
//	  - Returns error if response processing fails (does not interrupt FDO flow)
//
//	Integration Points:
//	  - DI.SetCredentials (msg type 11): logs successful credential setup
func (m *DIMiddleware) ProcessResponse(ctx context.Context, resp *http.Response) error {
	// Only process DI protocol responses
	if !m.isDIResponse(resp) {
		return nil
	}

	// Extract message type from response headers
	msgType := resp.Header.Get("Message-Type")
	if msgType == "11" { // DI.SetCredentials response
		return m.handleDISetCredentials(ctx, resp)
	}

	return nil
}

// isDIRequest identifies DI protocol requests by URL pattern.
// DI requests follow the pattern /fdo/101/msg/10 or /fdo/101/msg/12.
func (m *DIMiddleware) isDIRequest(req *http.Request) bool {
	return strings.Contains(req.URL.Path, "/fdo/101/msg/") &&
		(strings.HasSuffix(req.URL.Path, "/10") || strings.HasSuffix(req.URL.Path, "/12"))
}

// isDIResponse identifies DI protocol responses by message type header.
// DI responses have Message-Type header set to "11" (SetCredentials) or "13" (Done).
func (m *DIMiddleware) isDIResponse(resp *http.Response) bool {
	msgType := resp.Header.Get("Message-Type")
	return msgType == "11" || msgType == "13" // DI.SetCredentials or DI.Done
}

// handleDIAppStart processes DI.AppStart requests to fetch product passports.
// When enabled, it extracts the product UUID from the request body and calls
// the passport service to retrieve product item information.
func (m *DIMiddleware) handleDIAppStart(ctx context.Context, req *http.Request) error {
	if !m.enableProductPassport || m.ledgerClient == nil {
		return nil
	}

	// Read request body to extract product information
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	req.Body = io.NopCloser(strings.NewReader(string(body))) // Restore body for backend

	// Extract product UUID from CBOR body
	// Note: This is a simplified implementation - production code would need proper CBOR parsing
	productID := m.extractProductID(body)
	if productID == "" {
		return nil
	}

	// Fetch product item passport from external service
	passport, err := m.ledgerClient.GetProductItemPassport(ctx, productID)
	if err != nil {
		slog.Warn("Failed to get product passport", "product_id", productID, "error", err)
		return nil // Don't fail the request - passport lookup is optional
	}

	slog.Info("Retrieved product item passport",
		"uuid", passport.UUID,
		"records", len(passport.Records))

	return nil
}

// handleDISetCredentials logs successful DI credential setup.
// This provides visibility into the DI protocol completion.
func (m *DIMiddleware) handleDISetCredentials(ctx context.Context, resp *http.Response) error {
	slog.Info("DI.SetCredentials completed successfully")
	return nil
}

// extractProductID parses the product UUID from DI.AppStart request body.
// This is a placeholder implementation - production code would need:
// 1. Proper CBOR parsing of the request body
// 2. Extraction of device manufacturing info
// 3. Location of the productId field within the CBOR structure
func (m *DIMiddleware) extractProductID(body []byte) string {
	// Simple string search as placeholder
	// TODO: Implement proper CBOR parsing
	if strings.Contains(string(body), "productId") {
		// Extract actual product ID from CBOR data
		return "example-product-id"
	}

	return ""
}
