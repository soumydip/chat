# Chat backend

This directory contains the Go backend for the chat application. The server
uses Gin for HTTP routing and connects to PostgreSQL and Redis during startup.

## Requirements

- Go 1.27.1 or a compatible newer Go version
- PostgreSQL
- Redis
- [Air](https://github.com/air-verse/air) (optional, for live reload)

## Configuration

The application reads configuration from a `.env` file in this directory or
from system environment variables. Create `server/.env` with values similar to
the following:

```env
PORT=8080
DATABASE_URL=postgres://username:password@localhost:5432/chat?sslmode=disable
REDIS_URL=localhost:6379
JWT_SECRET=replace-with-a-long-random-secret
```

`PORT` defaults to `8080`, and `REDIS_URL` defaults to `localhost:6379`.
`DATABASE_URL` must be set to a valid PostgreSQL connection string. Set a
strong, unique value for `JWT_SECRET` before using the backend outside local
development.

Do not commit `.env` or any credentials to source control.

## Install dependencies

From this directory, download the Go module dependencies:

```bash
go mod download
```

Make sure PostgreSQL and Redis are running before starting the server. The
application checks both connections during initialization and exits if either
connection fails.

## Run the server

Start the backend with:

```bash
go run .
```

The server listens on `http://localhost:8080` by default. If you configure a
different `PORT`, use that port in the URL.

## Development with live reload

Install Air once:

```bash
go install github.com/air-verse/air@latest
```

Then run Air from the `server` directory:

```bash
air
```

The repository includes an `.air.toml` configuration for Windows and other
supported platforms.

## API

### Health check

```http
GET /health
```

Example response:

```json
{
  "status": "ok",
  "message": "Chat server is running smoothly!"
}
```

## Build

Create a production binary with:

```bash
go build -o chat-server .
```

Run the built binary from the `server` directory so it can load `.env`:

```bash
./chat-server
```
