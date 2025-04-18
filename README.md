# Telegram Authentication Client

A simple application that authenticates with Telegram, saves a session file, and exits.

## Prerequisites

1. You need to have Go installed (version 1.18 or later).
2. You need to obtain Telegram API credentials:
   - Visit [my.telegram.org/apps](https://my.telegram.org/apps)
   - Create a new application
   - Note your `app_id` and `app_hash`

## Usage

### Using Command-Line Flags

```bash
go run main.go --app-id=YOUR_APP_ID --app-hash=YOUR_APP_HASH --phone=YOUR_PHONE_NUMBER
```

For example:
```bash
go run main.go --app-id=12345 --app-hash=abcdef1234567890abcdef --phone=+1234567890
```

### Using Environment Variables

```bash
export APP_ID=YOUR_APP_ID
export APP_HASH=YOUR_APP_HASH
export PHONE=YOUR_PHONE_NUMBER
go run main.go
```

### Optional Parameters

- `--session-file`: Path to the session file (default: "tg-session.json" in the current directory)

## What This Does

1. Connects to Telegram using your API credentials
2. Requests an authentication code to be sent to your phone
3. Asks you to enter the code you received
4. Authenticates with Telegram
5. Saves the session file to the specified path
6. Exits

The session file can be used for subsequent authentication without needing to enter a code again.

## Notes

- Phone number should be in international format (e.g., +1234567890)
- If you have two-factor authentication enabled, this application will handle it properly
- The session file contains sensitive authentication information, keep it secure 