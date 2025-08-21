# FDO Server Proxy

A **reverse proxy** that runs the [go-fdo](https://github.com/fido-device-onboard/go-fdo) server as a backend process and intercepts HTTP requests to inject external passport service API calls. This approach allows you to extend FDO functionality without modifying the original FDO code.

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   FDO Client    │    │  FDO Proxy       │    │  Passport       │
│                 │    │  (Interceptor)   │    │  Service        │
│                 │◄──►│                  │◄──►│                 │
│                 │    │  • Request       │    │  • Product      │
│                 │    │    Interception  │    │    Item         │
│                 │    │  • Response      │    │    Passports    │
│                 │    │    Interception  │    │  • Commissioning│
│                 │    │                  │    │    Passports    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌─────────────────┐
                       │  FDO Server     │
                       │  (Backend)      │
                       │                 │
                       │  • DI Protocol  │
                       │  • TO0 Protocol │
                       │  • TO1 Protocol │
                       │  • TO2 Protocol │
                       └─────────────────┘
```

## Key Benefits

1. **No FDO Code Modification**: The original FDO server runs unchanged as a backend process
2. **Clean Separation**: Proxy handles external integrations, FDO server handles protocol logic
3. **Easy Deployment**: Just run the proxy instead of the FDO server directly
4. **Extensible**: Add new middleware for different integrations without touching FDO code
5. **Graceful Degradation**: If passport service is unavailable, FDO protocols continue to work

## Features

- **Product Item Passport Integration (DI Protocol)**: Intercepts DI.AppStart requests to retrieve product item passports from external service
- **Commissioning Passport Creation (TO2 Protocol)**: Intercepts TO2.Done2 responses to create commissioning passports in external service
- **Middleware Architecture**: Easy to add new request/response interceptors
- **Graceful Shutdown**: Properly stops both proxy and backend FDO server

## Installation

1. Clone this repository:
```bash
git clone <your-repo-url>
cd fdo-server-wrapper
```

2. Ensure you have the go-fdo repository available locally:
```bash
# The proxy expects go-fdo to be available at ../go-fdo
# Make sure the path exists relative to this repository
```

3. Build the proxy:
```bash
go build -o fdo-proxy ./cmd/server
```

## Usage

### Basic Usage (No Passport Integration)

```bash
./fdo-proxy -listen localhost:8080
```

This will:
1. Start the FDO server on `localhost:8081` as a backend process
2. Start the proxy on `localhost:8080` that forwards requests to the backend
3. All FDO protocols work normally without any external integrations

### With Passport Service Integration

```bash
./fdo-proxy \
  -listen localhost:8080 \
  -product-base-url https://cmulk1.cymanii.org:8443 \
  -commissioning-url http://cmulk1.cymanii.org:8000/create-commissioning-passport \
  -ca-cert ./certs/passport-service.pem \
  -client-cert ./certs/ucse-agent.crt \
  -client-key ./certs/ucse-agent.pem \
  -enable-product-passport \
  -owner-id your-owner-id
```

This will:
1. Start the FDO server on `localhost:8081` as a backend process
2. Start the proxy on `localhost:8080` with middleware enabled
3. Intercept DI.AppStart requests to retrieve product item passports via mTLS
4. Intercept TO2.Done2 responses to create commissioning passports

### Command Line Options

#### Proxy Options
- `-listen`: Address to listen on (default: localhost:8080)
- `-fdo-path`: Path to go-fdo repository (default: ../go-fdo)
- `-debug`: Enable debug logging

#### Passport Service Options
- `-product-base-url`: Base URL for product item passport service (e.g., https://cmulk1.cymanii.org:8443)
- `-commissioning-url`: URL for commissioning passport creation (e.g., http://cmulk1.cymanii.org:8000/create-commissioning-passport)
- `-ca-cert`: Path to CA cert PEM for product passport mTLS
- `-client-cert`: Path to client cert PEM for product passport mTLS
- `-client-key`: Path to client key PEM for product passport mTLS
- `-enable-product-passport`: Enable product item passport lookup during DI
- `-owner-id`: Owner ID for commissioning passports

## How It Works

### Request Flow

1. **FDO Client** sends request to proxy (e.g., `POST /fdo/101/msg/10`)
2. **Proxy** processes request through middleware:
   - DI middleware checks if it's a DI.AppStart request
   - If enabled, extracts product UUID and calls passport service
3. **Proxy** forwards request to backend FDO server
4. **FDO Server** processes the request normally
5. **FDO Server** sends response back to proxy
6. **Proxy** processes response through middleware:
   - TO2 middleware checks if it's a TO2.Done2 response
   - If enabled, extracts device GUID and creates commissioning passport
7. **Proxy** sends response back to FDO Client

### Middleware Integration Points

#### DI Protocol (Message Type 10)
- **Request Interception**: Extracts product UUID from DI.AppStart request body
- **Passport Service Call**: `GET {base}/product_item/?uuid={uuid}` with mTLS
- **Logging**: Logs retrieved product item passport information

#### TO2 Protocol (Message Type 71)
- **Response Interception**: Extracts device GUID from TO2.Done2 response
- **Passport Service Call**: `POST {commissioning-url}` with JSON payload
- **Logging**: Logs created commissioning passport information

## API Integration

### Product Item Passport API

The proxy calls the passport service:

```
GET {base}/product_item/?uuid={uuid}
```

**Headers**: mTLS with provided CA, client cert, and key

**Response:**
```json
{
  "schema_version": 0.1,
  "uuid": "191e886b-dfff-4f39-9618-d7a364ec0c90",
  "records": [
    {
      "uuid": "82a954d6-5090-4789-9bf9-ff7b591b5224",
      "signature": "MEUCIBrdQUuxUsFFrj9qW61RHiKfsdvWaJVnkvSU57P+7H9LAiEAwoV3WL1dGPBsDrssWsa5mKM25WlB71Ik+iHMQ2uQLhg=",
      "descriptor": "PRODUCT PASSPORT"
    }
  ],
  "metadata": {
    "version": "1.0",
    "creation_time": "1754331025571481856",
    "board_sn": "d8b976ff7bac6ede3c0b3ed4de15f288de3ab18df68ad74f157cfbdc09d49732"
  },
  "agent": {
    "uuid": "100ace34-3402-4ca9-a692-f7eda6c2834d",
    "signature": "MEUCIQCMyl/opKsfUTm2v1oGHdLPZRFsRIIXmElABc/bOOKVigIgCaHWZ9rZcu71/2guNJviSw6Hr2pf0fLdRfzaIZC3koQ="
  },
  "signature": "MEQCIDk1TJ/MBUgagAWnh2vRwk8X7sorQUmfVBRrAlP7gAqOAiBwrI+EUVn+mdVqFUgmPgdNedtgn4bs/roVD6ElCLHNZQ=="
}
```

### Commissioning Passport API

The proxy creates commissioning passports via:

```
POST {commissioning-url}
```

**Headers**: `Content-Type: application/json`

**Request Body:**
```json
{
  "controller_uuid": "191e886b-dfff-4f39-9618-d7a364ec0c90",
  "cert": "string",
  "deployed_location": "string",
  "timestamp": "1754509904342152960"
}
```

## Error Handling

- **Passport service failures do not interrupt FDO protocols**: If the passport service is unavailable or returns errors, the proxy logs warnings but allows the FDO protocol to continue
- **Graceful degradation**: The proxy can run without passport integration if the service is not configured
- **Backend server failures**: If the FDO server fails to start or becomes unavailable, the proxy will return appropriate HTTP errors

## Development

### Project Structure

```
fdo-server-wrapper/
├── cmd/
│   └── server/
│       └── main.go          # Main proxy entry point
├── internal/
│   ├── ledger/
│   │   └── client.go        # Passport service client
│   ├── middleware/
│   │   ├── di.go           # DI protocol middleware
│   │   └── to2.go          # TO2 protocol middleware
│   └── proxy/
│       └── server.go        # Reverse proxy implementation
├── go.mod                   # Go module definition
├── Makefile                 # Build and development tools
├── README.md               # This file
└── .gitignore              # Git ignore rules
```

### Adding New Middleware

To add new functionality:

1. **Create new middleware** in `internal/middleware/`:
```go
type NewMiddleware struct {
    // Your middleware fields
}

func (m *NewMiddleware) ProcessRequest(ctx context.Context, req *http.Request) error {
    // Process request
    return nil
}

func (m *NewMiddleware) ProcessResponse(ctx context.Context, resp *http.Response) error {
    // Process response
    return nil
}
```

2. **Add to main.go**:
```go
newMiddleware := middleware.NewNewMiddleware(config)
middlewareList = append(middlewareList, newMiddleware)
```

3. **Add configuration flags** as needed

### Testing

```bash
# Run with debug logging
./fdo-proxy -listen localhost:8080 -debug

# Test with passport integration
./fdo-proxy \
  -listen localhost:8080 \
  -product-base-url https://cmulk1.cymanii.org:8443 \
  -commissioning-url http://cmulk1.cymanii.org:8000/create-commissioning-passport \
  -ca-cert ./certs/passport-service.pem \
  -client-cert ./certs/ucse-agent.crt \
  -client-key ./certs/ucse-agent.pem \
  -enable-product-passport \
  -owner-id test-owner \
  -debug
```

## Deployment

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o fdo-proxy ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/fdo-proxy .
CMD ["./fdo-proxy", "-listen", ":8080"]
```

### Systemd Service

```ini
[Unit]
Description=FDO Server Proxy
After=network.target

[Service]
Type=simple
User=fdo
WorkingDirectory=/opt/fdo-proxy
ExecStart=/opt/fdo-proxy/fdo-proxy -listen :8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## License

This project is licensed under the MIT License. See the LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

### Development Setup

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Follow Go formatting standards (`gofmt`)
- Add tests for new functionality
- Update documentation as needed
- Keep the thin proxy layer design principle in mind 