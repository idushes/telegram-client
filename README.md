# Telegram MTProto Client with MCP Server

This application implements a Telegram MTProto client that exposes its functionality through a Model Control Protocol (MCP) server over Server-Sent Events (SSE).

## Features

- MCP server with SSE interface for controlling Telegram client
- Authentication handling with PIN code via MCP tools
- Automatic retry with 5-second delay on authentication failure
- Session persistence between restarts
- Environment-based configuration
- Docker support for easy deployment
- Modular code structure for better maintainability

## Project Structure

```
telegram-client/
├── mcp/
│   ├── server.go  # Core server structure and initialization
│   ├── handlers.go # MCP tool handlers
│   └── auth.go    # Telegram authentication functions
├── main.go        # Application entry point
├── session/       # Directory for storing session data
```

## Environment Variables

The application requires the following environment variables:

| Variable        | Description                                         | Required |
|-----------------|-----------------------------------------------------|----------|
| MCP_SERVER_PORT | Port for the MCP server                             | Yes      |
| PHONE           | Phone number for Telegram authentication            | Yes      |
| APP_ID          | Telegram API App ID                                 | Yes      |
| APP_HASH        | Telegram API App Hash                               | Yes      |

## MCP Tools

### Authentication Tools

- `telegram_send_code`: Send authentication code
  - Parameters: `{"code": "12345"}`
  - Response: Success/Error message

## Authentication Flow

1. The application attempts to authenticate with Telegram on startup
2. If a code is required, the application waits for the `telegram_send_code` tool to be called
3. If authentication fails, the application retries after a 5-second delay


This will:
- Build the application
- Set the environment variables with your credentials
- Start the MCP server on port 8080


## Error Handling

- If `MCP_SERVER_PORT` is not provided, the application will fail to start
- If Telegram credentials are not provided, the application will fail to start
- Authentication errors will trigger a retry after 5 seconds
- If the authentication code is not being requested, calling `telegram_send_code` will result in an error

## Libraries Used

- [gotd/td](https://github.com/gotd/td) - Telegram MTProto client implementation in Go
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - Model Control Protocol implementation in Go 

## Session Storage

The application stores Telegram session data in the `session/` directory. The session files are named using an MD5 hash of the phone number. This allows the application to reuse authenticated sessions between restarts, avoiding the need to re-authenticate each time.

When running with Docker, the session directory is mounted as a volume to ensure persistence between container restarts. 