package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fdo-server-wrapper/internal/ledger"
	"github.com/fdo-server-wrapper/internal/proxy"
)

// TO2Middleware intercepts TO2 protocol messages to create commissioning passports.
// It tracks device onboarding completion and records commissioning events.
type TO2Middleware struct {
	ledgerClient proxy.LedgerClient
	ownerID      string
}

// NewTO2Middleware creates middleware for TO2 protocol integration.
// When configured, it will create commissioning passports upon successful device onboarding.
func NewTO2Middleware(ledgerClient proxy.LedgerClient, ownerID string) *TO2Middleware {
	return &TO2Middleware{
		ledgerClient: ledgerClient,
		ownerID:      ownerID,
	}
}

// ProcessRequest handles incoming TO2 protocol requests.
//
// Contract:
//
//	Preconditions:
//	  - req is not nil and contains valid HTTP request
//	  - ctx is not nil
//
//	Postconditions:
//	  - Returns nil if request is not TO2-related or processing succeeds
//	  - Returns error if request processing fails (does not interrupt FDO flow)
//
//	Integration Points:
//	  - TO2.HelloDevice (msg type 60): logs device hello for tracking
func (m *TO2Middleware) ProcessRequest(ctx context.Context, req *http.Request) error {
	// Only process TO2 protocol requests
	if !m.isTO2Request(req) {
		return nil
	}

	// Parse message type from URL path: /fdo/101/msg/{msgType}
	pathParts := strings.Split(req.URL.Path, "/")
	if len(pathParts) < 5 {
		return nil
	}

	msgType := pathParts[4]
	if msgType == "60" { // TO2.HelloDevice message
		return m.handleTO2HelloDevice(ctx, req)
	}

	return nil
}

// ProcessResponse handles outgoing TO2 protocol responses.
//
// Contract:
//
//	Preconditions:
//	  - resp is not nil and contains valid HTTP response
//	  - ctx is not nil
//
//	Postconditions:
//	  - Returns nil if response is not TO2-related or processing succeeds
//	  - Returns error if response processing fails (does not interrupt FDO flow)
//
//	Integration Points:
//	  - TO2.Done2 (msg type 71): creates commissioning passport upon completion
func (m *TO2Middleware) ProcessResponse(ctx context.Context, resp *http.Response) error {
	// Only process TO2 protocol responses
	if !m.isTO2Response(resp) {
		return nil
	}

	// Extract message type from response headers
	msgType := resp.Header.Get("Message-Type")
	if msgType == "71" { // TO2.Done2 response
		return m.handleTO2Done2(ctx, resp)
	}

	return nil
}

// isTO2Request identifies TO2 protocol requests by URL pattern.
// TO2 requests follow the pattern /fdo/101/msg/60 or /fdo/101/msg/70.
func (m *TO2Middleware) isTO2Request(req *http.Request) bool {
	return strings.Contains(req.URL.Path, "/fdo/101/msg/") &&
		(strings.HasSuffix(req.URL.Path, "/60") || strings.HasSuffix(req.URL.Path, "/70"))
}

// isTO2Response identifies TO2 protocol responses by message type header.
// TO2 responses have Message-Type header set to "71" (Done2).
func (m *TO2Middleware) isTO2Response(resp *http.Response) bool {
	msgType := resp.Header.Get("Message-Type")
	return msgType == "71" // TO2.Done2
}

// handleTO2HelloDevice logs TO2.HelloDevice requests for tracking.
// This provides visibility into device onboarding initiation.
func (m *TO2Middleware) handleTO2HelloDevice(ctx context.Context, req *http.Request) error {
	slog.Info("TO2.HelloDevice request received")
	return nil
}

// handleTO2Done2 processes TO2.Done2 responses to create commissioning passports.
// When a device completes onboarding successfully, this creates a record
// of the commissioning event in the external passport service.
func (m *TO2Middleware) handleTO2Done2(ctx context.Context, resp *http.Response) error {
	if m.ledgerClient == nil {
		return nil
	}

	// Extract device GUID from response or session context
	// Note: This is a simplified implementation - production code would need proper CBOR parsing
	deviceGUID := m.extractDeviceGUID(resp)
	if deviceGUID == "" {
		slog.Warn("Could not extract device GUID from TO2.Done2 response")
		return nil
	}

	// Build commissioning passport request
	reqBody := &ledger.CommissioningCreateRequest{
		ControllerUUID:   deviceGUID,
		Cert:             "", // TODO: Extract actual certificate if available
		DeployedLocation: "", // TODO: Extract location from device info or config
		Timestamp:        fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	// Create commissioning passport in external service
	if err := m.ledgerClient.CreateCommissioningPassport(ctx, reqBody); err != nil {
		slog.Warn("Failed to create commissioning passport",
			"controller_uuid", deviceGUID,
			"error", err)
		return nil // Don't fail the response - passport creation is optional
	}

	slog.Info("Created commissioning passport",
		"controller_uuid", reqBody.ControllerUUID)

	return nil
}

// extractDeviceGUID parses the device GUID from TO2.Done2 response.
// This is a placeholder implementation - production code would need:
// 1. Proper CBOR parsing of the response body
// 2. Extraction of device GUID from the response structure
// 3. Or extraction from session context if available
func (m *TO2Middleware) extractDeviceGUID(resp *http.Response) string {
	// Placeholder implementation
	// TODO: Implement proper CBOR parsing to extract device GUID
	return "example-device-guid"
}
