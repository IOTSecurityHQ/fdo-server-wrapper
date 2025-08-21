package ledger

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Client is a small helper around the two passport endpoints used by the proxy.
// Keeps the layer thin and avoids unnecessary abstractions.
type Client struct {
	productBaseURL    string
	commissioningURL  string
	productHTTP       *http.Client
	commissioningHTTP *http.Client
}

// NewClient configures clients for:
// - Product item passport (mTLS GET)
// - Commissioning passport (HTTP POST)
func NewClient(productBaseURL, commissioningURL, caCertPath, clientCertPath, clientKeyPath string) (*Client, error) {
	productHTTP, err := newMTLSHTTPClient(caCertPath, clientCertPath, clientKeyPath)
	if err != nil {
		return nil, err
	}

	return &Client{
		productBaseURL:    productBaseURL,
		commissioningURL:  commissioningURL,
		productHTTP:       productHTTP,
		commissioningHTTP: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func newMTLSHTTPClient(caPath, certPath, keyPath string) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("append CA cert")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      caPool,
			Certificates: []tls.Certificate{cert},
		},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

// Shapes below mirror the service responses closely.
type ProductItemPassport struct {
	SchemaVersion float64             `json:"schema_version"`
	UUID          string              `json:"uuid"`
	Records       []ProductItemRecord `json:"records"`
	Metadata      ProductItemMetadata `json:"metadata"`
	Agent         ProductItemAgent    `json:"agent"`
	Signature     string              `json:"signature"`
}

type ProductItemRecord struct {
	UUID       string `json:"uuid"`
	Signature  string `json:"signature"`
	Descriptor string `json:"descriptor"`
}

type ProductItemMetadata struct {
	Version      string `json:"version"`
	CreationTime string `json:"creation_time"`
	BoardSN      string `json:"board_sn"`
}

type ProductItemAgent struct {
	UUID      string `json:"uuid"`
	Signature string `json:"signature"`
}

// GetProductItemPassport retrieves a product item passport from the external service.
//
// Contract:
//
//	  Preconditions:
//	    - ctx is not nil
//	    - uuid is a non-empty string
//	    - productBaseURL is configured
//	    - mTLS certificates are valid and accessible
//
//	  Postconditions:
//	    - Returns ProductItemPassport with schema_version, uuid, records, metadata, agent, signature
//	    - Returns nil passport and error if service unavailable or invalid response
//
//	  Error Conditions:
//	    - Network errors: connection failures, timeouts
//	    - TLS errors: invalid certificates, mTLS handshake failures
//	    - HTTP errors: non-200 status codes
//	    - JSON errors: malformed response body
//
//		GET {productBaseURL}/product_item/?uuid={uuid}
//
// Uses mTLS with the configured CA, client cert, and key.
func (c *Client) GetProductItemPassport(ctx context.Context, uuid string) (*ProductItemPassport, error) {
	if c.productBaseURL == "" {
		return nil, fmt.Errorf("product base URL not configured")
	}

	u, err := url.Parse(c.productBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	u.Path = "/product_item/"
	q := u.Query()
	q.Set("uuid", uuid)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.productHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("passport GET status %d: %s", resp.StatusCode, string(b))
	}

	var out ProductItemPassport
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

// CommissioningCreateRequest is the payload the service expects.
type CommissioningCreateRequest struct {
	ControllerUUID   string `json:"controller_uuid"`
	Cert             string `json:"cert"`
	DeployedLocation string `json:"deployed_location"`
	Timestamp        string `json:"timestamp"`
}

// CreateCommissioningPassport creates a commissioning passport in the external service.
//
// Contract:
//
//	  Preconditions:
//	    - ctx is not nil
//	    - body is not nil and contains valid commissioning data
//	    - commissioningURL is configured
//	    - body.ControllerUUID is non-empty
//	    - body.Timestamp is a valid timestamp string
//
//	  Postconditions:
//	    - Returns nil on successful creation (HTTP 2xx status)
//	    - Returns error on failure (HTTP 4xx/5xx status or network errors)
//
//	  Error Conditions:
//	    - Network errors: connection failures, timeouts
//	    - HTTP errors: non-2xx status codes
//	    - JSON errors: malformed request body
//	    - Validation errors: missing required fields
//
//		POST {commissioningURL}
func (c *Client) CreateCommissioningPassport(ctx context.Context, body *CommissioningCreateRequest) error {
	if c.commissioningURL == "" {
		return fmt.Errorf("commissioning URL not configured")
	}

	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.commissioningURL, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.commissioningHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("commissioning POST status %d: %s", resp.StatusCode, string(bb))
	}
	return nil
}
