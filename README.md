# PepeAPI

PepeAPI is a small backend service written in Go.

The project is being developed as a practical backend project for learning how real Go services are structured: HTTP handling, PostgreSQL integration, testing, structured logging, graceful shutdown, and observability.

The goal is to gradually evolve the project from a simple HTTP API into a production-like Go service.

## Features

Currently implemented:

- HTTP API using the Go standard library
- PostgreSQL integration
- Repository layer for database access
- Graceful HTTP server shutdown
- Structured logging with `log/slog`
- Unit testing with `httptest` and GoMock
- Docker image build
- Context propagation from HTTP handlers to the database layer

## Tech Stack

- Go 1.26
- `net/http`
- PostgreSQL
- `pgx/v5`
- `log/slog`
- GoMock
- `httptest`
- Docker

## Project Structure

```text
.
├── cmd/
│   └── server/
│       ├── main.go
│       ├── main_test.go
│       └── item_repository_mock_test.go
│
├── internal/
│   └── database/
│       ├── item.go
│       └── postgres.go
│
├── Dockerfile
├── go.mod
├── go.sum
└── README.md