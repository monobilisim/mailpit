# API Relay Configuration

Mailpit can be configured to forward incoming messages to an external API endpoint via HTTP POST requests. This is useful for integrating with external services that need to process or store email messages.

## Configuration Options

### Command Line Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--api-relay-enabled` | Enable API relaying of messages | `--api-relay-enabled` |
| `--api-relay-config` | Path to API relay configuration file | `--api-relay-config /path/to/config.yaml` |
| `--api-relay-endpoint` | API endpoint to relay messages to | `--api-relay-endpoint https://api.example.com/email` |
| `--api-relay-auth-type` | Authentication type (`basic`, `bearer`, `api-key`) | `--api-relay-auth-type bearer` |
| `--api-relay-auth-token` | Authentication token (for bearer or api-key auth) | `--api-relay-auth-token your-token-here` |
| `--api-relay-auth-username` | Username for basic auth | `--api-relay-auth-username user` |
| `--api-relay-auth-password` | Password for basic auth | `--api-relay-auth-password pass` |
| `--api-relay-api-key-header` | Header name for API key authentication (default: X-API-Key) | `--api-relay-api-key-header X-Server-API-Key` |
| `--api-relay-timeout` | Request timeout in seconds | `--api-relay-timeout 30` |
| `--api-relay-insecure-skip-verify` | Skip TLS certificate verification | `--api-relay-insecure-skip-verify` |

### Environment Variables

| Environment Variable | Description | Example |
|----------------------|-------------|---------|
| `MP_API_RELAY_ENABLED` | Enable API relaying (set to any value) | `MP_API_RELAY_ENABLED=true` |
| `MP_API_RELAY_CONFIG` | Path to API relay configuration file | `MP_API_RELAY_CONFIG=/path/to/config.yaml` |
| `MP_API_RELAY_ENDPOINT` | API endpoint to relay messages to | `MP_API_RELAY_ENDPOINT=https://api.example.com/email` |
| `MP_API_RELAY_AUTH_TYPE` | Authentication type (`basic`, `bearer`, `api-key`) | `MP_API_RELAY_AUTH_TYPE=bearer` |
| `MP_API_RELAY_AUTH_TOKEN` | Authentication token (for bearer or api-key auth) | `MP_API_RELAY_AUTH_TOKEN=your-token-here` |
| `MP_API_RELAY_AUTH_USERNAME` | Username for basic auth | `MP_API_RELAY_AUTH_USERNAME=user` |
| `MP_API_RELAY_AUTH_PASSWORD` | Password for basic auth | `MP_API_RELAY_AUTH_PASSWORD=pass` |
| `MP_API_RELAY_API_KEY_HEADER` | Header name for API key authentication (default: X-API-Key) | `MP_API_RELAY_API_KEY_HEADER=X-Server-API-Key` |
| `MP_API_RELAY_TIMEOUT` | Request timeout in seconds | `MP_API_RELAY_TIMEOUT=30` |
| `MP_API_RELAY_INSECURE_SKIP_VERIFY` | Skip TLS certificate verification (set to any value) | `MP_API_RELAY_INSECURE_SKIP_VERIFY=true` |

### Configuration File

You can also configure the API relay using a YAML configuration file. Here's an example:

```yaml
# API Relay Configuration Example
# Save this file and reference it with --api-relay-config or set MP_API_RELAY_CONFIG environment variable

# Enable the API relay (default: false)
enabled: true

# The endpoint to send HTTP POST requests to
endpoint: "https://api.example.com/email"

# Authentication type: "basic", "bearer", "api-key", or "" for no auth
auth-type: "bearer"

# Optional request template (Go template syntax)
request-template:
  template: |
    {
      "from": "{{.From}}",
      "to": {{.To | toJson}},
      "subject": "{{index (index .Headers "Subject") 0}}",
      "text": "{{.Raw | escapeJson}}",
      "html": "<p>{{.Raw | escapeJson}}</p>"
    }
  content-type: "application/json"

# Request timeout in seconds (default: 30)
timeout: 30

# Skip TLS certificate verification (default: false)
insecure-skip-verify: false
```

## Request Format

By default, the API relay sends a JSON payload with the following structure:

```json
{
  "from": "sender@example.com",
  "to": ["recipient1@example.com", "recipient2@example.com"],
  "message": "base64-encoded-raw-email"
}
```

### Custom Request Templates

You can customize the request body format using Go templates. The following variables are available in the template:

- `.From`: Sender's email address
- `.To`: Array of recipient email addresses
- `.Message`: Base64-encoded raw email message
- `.Raw`: Raw email message as a string
- `.Headers`: Map of email headers (values are string arrays)

Template functions:
- `toJson`: Convert a value to JSON
- `escapeJson`: Escape a string for JSON

Example template for Postal API:

```yaml
request-template:
  template: |
    {
      "mail_from": "{{.From}}",
      "rcpt_to": {{.To | toJson}},
      "message": "{{.Message}}"
    }
  content-type: "application/json"
```

## Response Handling
### Response Handling

The API endpoint should return a `200 OK` status code if the message was processed successfully. Any other status code will be considered an error, and Mailpit will log the error message.

## Example Usage

### Basic Configuration

```bash
mailpit --api-relay-enabled --api-relay-endpoint https://api.example.com/email
```

### With Bearer Token Authentication

```bash
mailpit --api-relay-enabled \
  --api-relay-endpoint https://api.example.com/email \
  --api-relay-auth-type bearer \
  --api-relay-auth-token "your-bearer-token"
```

### Using a Configuration File

1. Create a configuration file (e.g., `api-relay.yaml`):

```yaml
enabled: true
endpoint: "https://api.example.com/email"
auth-type: "bearer"
auth-token: "your-bearer-token"
timeout: 30
insecure-skip-verify: false
```

2. Start Mailpit with the configuration file:

```bash
mailpit --api-relay-config /path/to/api-relay.yaml
```

## Error Handling

If the API request fails, Mailpit will log the error and continue processing. The message will still be stored in Mailpit's database unless there's an error during that process.

Common errors include:
- Network connectivity issues
- Invalid or expired authentication credentials
- Server errors (5xx) from the API endpoint
- Timeout waiting for a response

## Security Considerations

- Always use HTTPS for the API endpoint to ensure message confidentiality
- Use authentication to prevent unauthorized access to your API endpoint
- Consider rate limiting on your API endpoint to prevent abuse
- Use the `insecure-skip-verify` option only in development environments
