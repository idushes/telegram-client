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
- Option to store session data in ETCD (key/value store) instead of local filesystem

## Project Structure

```
telegram-client/
├── mcp/
│   ├── server.go     # Core server structure and initialization
│   ├── handlers.go   # MCP tool handlers
│   ├── auth.go       # Telegram authentication functions
│   └── etcd_storage.go # ETCD session storage implementation
├── main.go           # Application entry point
├── session/          # Directory for storing session data (when not using ETCD)
```

## Environment Variables

The application requires the following environment variables:

| Variable        | Description                                         | Required |
|-----------------|-----------------------------------------------------|----------|
| MCP_SERVER_PORT | Port for the MCP server                             | Yes      |
| PHONE           | Phone number for Telegram authentication            | Yes      |
| APP_ID          | Telegram API App ID                                 | Yes      |
| APP_HASH        | Telegram API App Hash                               | Yes      |
| ETCD_ENDPOINT   | ETCD HTTP API endpoint for session storage          | No       |

## Session Storage

The application supports two types of session storage:

1. **File Storage (default)**: Session data is stored in the `session/` directory in the local filesystem.
2. **ETCD Storage**: If the `ETCD_ENDPOINT` environment variable is set, session data is stored in ETCD using HTTP API.

To use ETCD storage, set the `ETCD_ENDPOINT` environment variable to your ETCD HTTP API endpoint:

```sh
export ETCD_ENDPOINT="http://etcd-server:2379/v3/kv"
```

The ETCD key prefix used is `telegram/sessions/` followed by an MD5 hash of the phone number.

## MCP Tools

### Authentication Tools

- `send_code`: Send authentication code
  - Parameters: `{"code": "12345"}`
  - Response: Success/Error message

### Information Tools

- `get_groups`: Get list of Telegram groups
  - Parameters: `{"limit": 50}` (optional, defaults to 50)
  - Response: JSON object with list of groups and count
  - Example response: 
    ```json
    {
      "groups": [
        {
          "id": 12345678,
          "title": "Group Name",
          "type": "megagroup",
          "username": "groupname",
          "members": 100,
          "verified": false,
          "restricted": false
        }
      ],
      "count": 1
    }
    ```

- `get_group_messages`: Get messages from a Telegram group by ID
  - Parameters: 
    - `group_id`: ID of the group (required, can be a positive or negative number)
    - `limit`: Maximum number of messages to return (optional, defaults to 20)
  - Response: JSON object with list of messages and count
  - Example response:
    ```json
    {
      "messages": [
        {
          "id": 12345,
          "date": 1634567890,
          "text": "Hello world!",
          "out": false,
          "mentioned": false,
          "media": false,
          "from": {
            "type": "user",
            "id": 123456789
          }
        },
        {
          "id": 12344,
          "date": 1634567880,
          "type": "service_message",
          "action": "user_added"
        }
      ],
      "count": 2,
      "group_id": 1789380160
    }
    ```
  - Note: To get the group ID, first use the `get_groups` tool to list all available groups

## Authentication Flow

1. The application attempts to authenticate with Telegram on startup
2. If a code is required, the application waits for the `send_code` tool to be called
3. If authentication fails, the application retries after a 5-second delay


This will:
- Build the application
- Set the environment variables with your credentials
- Start the MCP server on port 8080


## Error Handling

- If `MCP_SERVER_PORT` is not provided, the application will fail to start
- If Telegram credentials are not provided, the application will fail to start
- Authentication errors will trigger a retry after 5 seconds
- If the authentication code is not being requested, calling `send_code` will result in an error

## Libraries Used

- [gotd/td](https://github.com/gotd/td) - Telegram MTProto client implementation in Go
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - Model Control Protocol implementation in Go 

## Session Storage

The application supports two storage methods for Telegram session data:

1. **File Storage (default)**:
   - The application stores Telegram session data in the `session/` directory. 
   - The session files are named using an MD5 hash of the phone number.
   - This allows the application to reuse authenticated sessions between restarts.
   - When running with Docker, the session directory is mounted as a volume to ensure persistence.

2. **ETCD Storage**:
   - If `ETCD_ENDPOINT` is provided, the application stores sessions in ETCD via HTTP API.
   - This is useful for distributed or containerized environments.
   - The implementation uses base64 encoding for both keys and values.
   - Keys follow the pattern `telegram/sessions/{MD5_HASH_OF_PHONE}`. 