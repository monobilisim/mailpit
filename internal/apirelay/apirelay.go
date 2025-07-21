package apirelay

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/axllent/mailpit/config"
	"github.com/axllent/mailpit/internal/logger"
	"gopkg.in/yaml.v3"
)

// LoadConfig loads the API relay configuration from a YAML file
func LoadConfig(filename string) error {
	if filename == "" {
		logger.Log().Debug("[apirelay] No config file specified, using default configuration")
		return nil
	}

	logger.Log().Debugf("[apirelay] Loading configuration from %s", filename)

	// Get absolute path
	absPath, _ := filepath.Abs(filename)
	logger.Log().Debugf("[apirelay] Absolute config path: %s", absPath)

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		logger.Log().Errorf("[apirelay] Config file does not exist: %s", absPath)
		return fmt.Errorf("config file does not exist: %s", absPath)
	}

	data, err := ioutil.ReadFile(absPath)
	if err != nil {
		logger.Log().Errorf("[apirelay] Error reading config file: %v", err)
		return fmt.Errorf("error reading config file: %v", err)
	}

	logger.Log().Debugf("[apirelay] Read %d bytes from config file", len(data))

	// Create a temporary struct to handle the raw YAML
	type rawConfig struct {
		Enabled            bool              `yaml:"enabled"`
		Endpoint           string            `yaml:"endpoint"`
		AuthType           string            `yaml:"auth-type"`
		AuthToken          string            `yaml:"auth-token"`
		AuthUsername       string            `yaml:"auth-username"`
		AuthPassword       string            `yaml:"auth-password"`
		APIKeyHeader       string            `yaml:"api-key-header"`
		Headers            map[string]string `yaml:"headers"`
		Timeout            int               `yaml:"timeout"`
		InsecureSkipVerify bool              `yaml:"insecure-skip-verify"`
		RequestTemplate    struct {
			Template    string `yaml:"template"`
			ContentType string `yaml:"content-type"`
		} `yaml:"request-template"`
	}

	logger.Log().Debugf("[apirelay] Raw YAML content:\n%s", string(data))

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		logger.Log().Errorf("[apirelay] Error parsing YAML config: %v", err)
		return fmt.Errorf("error parsing config file: %v", err)
	}

	logger.Log().Debugf("[apirelay] Raw config loaded: %+v", raw)

	// Log raw config values for debugging
	logger.Log().Debugf("[apirelay] Raw config values: %+v", raw)

	// Set default values if not provided
	if raw.Timeout == 0 {
		raw.Timeout = 30 // Default 30 seconds
	}
	if raw.APIKeyHeader == "" {
		raw.APIKeyHeader = "X-API-Key"
	}

	// Convert to the config struct
	cfg := config.APIRelayConfigStruct{
		Enabled:            raw.Enabled,
		Endpoint:           raw.Endpoint,
		AuthType:           raw.AuthType,
		AuthToken:          raw.AuthToken,
		AuthUsername:       raw.AuthUsername,
		AuthPassword:       raw.AuthPassword,
		APIKeyHeader:       raw.APIKeyHeader,
		Headers:            raw.Headers,
		Timeout:            raw.Timeout,
		InsecureSkipVerify: raw.InsecureSkipVerify,
	}

	logger.Log().Debugf("[apirelay] Parsed config: %+v", cfg)

	// Only set RequestTemplate if template is not empty
	if raw.RequestTemplate.Template != "" {
		logger.Log().Debug("[apirelay] Using custom request template")
		cfg.RequestTemplate = &config.RequestTemplate{
			Template:    raw.RequestTemplate.Template,
			ContentType: raw.RequestTemplate.ContentType,
		}
	} else {
		logger.Log().Debug("[apirelay] No custom request template found, using default format")
	}

	config.APIRelayConfig = cfg

	return nil
}

// min returns the smaller of x or y
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// prepareRequestBody prepares the request body based on the configuration
func prepareRequestBody(from string, to []string, msg []byte) ([]byte, string, error) {
	// If a custom template is defined, use it
	if config.APIRelayConfig.RequestTemplate != nil && config.APIRelayConfig.RequestTemplate.Template != "" {
		// Create a function map with template functions
		funcMap := template.FuncMap{
			// toJson converts a value to its JSON representation
			"toJson": func(v interface{}) (string, error) {
				b, err := json.Marshal(v)
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
			// escapeJson converts a value to its JSON string representation (with quotes)
			"escapeJson": func(v interface{}) (string, error) {
				b, err := json.Marshal(v)
				if err != nil {
					return "", err
				}
				// Return as a JSON string (with quotes)
				return string(b), nil
			},
		}

		tmpl, err := template.New("request").Funcs(funcMap).Parse(config.APIRelayConfig.RequestTemplate.Template)
		if err != nil {
			return nil, "", fmt.Errorf("error parsing request template: %w", err)
		}

		// Parse the email message to extract headers
		msgReader := bytes.NewReader(msg)
		email, err := mail.ReadMessage(msgReader)
		headers := make(map[string]string)
		if err == nil {
			// Copy all headers to our map
			for k, v := range email.Header {
				if len(v) > 0 {
					headers[k] = v[0]
				}
			}
		} else {
			logger.Log().Warnf("[apirelay] Error parsing email headers: %v", err)
		}

		// Prepare template data
		data := map[string]interface{}{
			"From":    from,
			"To":      to,
			"Message": base64.StdEncoding.EncodeToString(msg),
			"Raw":     string(msg),
			"Headers": headers,
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, "", fmt.Errorf("error executing template: %w", err)
		}

		contentType := config.APIRelayConfig.RequestTemplate.ContentType
		if contentType == "" {
			contentType = "application/json"
		}

		return buf.Bytes(), contentType, nil
	}

	// Default JSON format
	reqBody := map[string]interface{}{
		"from":    from,
		"to":      to,
		"message": base64.StdEncoding.EncodeToString(msg),
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("error marshaling request body: %w", err)
	}

	return reqBodyBytes, "application/json", nil
}

// Relay sends an email message to the configured API endpoint
func Relay(from string, to []string, msg []byte) error {
	if !config.APIRelayConfig.Enabled {
		logger.Log().Debug("[apirelay] Relay is disabled, skipping")
		return nil
	}

	logger.Log().Debugf("[apirelay] Relay called - from: %s, to: %v, message size: %d bytes", from, to, len(msg))
	logger.Log().Debugf("[apirelay] Configuration - endpoint: %s, auth-type: %s", config.APIRelayConfig.Endpoint, config.APIRelayConfig.AuthType)

	if !config.APIRelayConfig.Enabled || config.APIRelayConfig.Endpoint == "" {
		return nil
	}

	// Prepare request body
	body, contentType, err := prepareRequestBody(from, to, msg)
	if err != nil {
		logger.Log().Errorf("[apirelay] Error preparing request body: %v", err)
		return fmt.Errorf("error preparing request body: %v", err)
	}

	logger.Log().Debugf("[apirelay] Request body prepared, size: %d bytes, content-type: %s", len(body), contentType)

	// Create request with timeout
	timeout := time.Duration(config.APIRelayConfig.Timeout) * time.Second

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "POST", config.APIRelayConfig.Endpoint, bytes.NewBuffer(body))
	if err != nil {
		logger.Log().Errorf("[apirelay] Error creating request: %v", err)
		return fmt.Errorf("error creating request: %v", err)
	}

	logger.Log().Debugf("[apirelay] Request created to: %s", config.APIRelayConfig.Endpoint)

	// Set default headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Mailpit/dev")

	// Add custom headers
	headers := make([]string, 0, len(config.APIRelayConfig.Headers))
	for k, v := range config.APIRelayConfig.Headers {
		req.Header.Set(k, v)
		headers = append(headers, fmt.Sprintf("%s: %s", k, v))
	}

	logger.Log().Debugf("[apirelay] Request headers: %v", headers)

	// Set auth headers
	switch strings.ToLower(config.APIRelayConfig.AuthType) {
	case "basic":
		req.SetBasicAuth(config.APIRelayConfig.AuthUsername, config.APIRelayConfig.AuthPassword)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+config.APIRelayConfig.AuthToken)
	case "api-key":
		headerName := config.APIRelayConfig.APIKeyHeader
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, config.APIRelayConfig.AuthToken)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: config.APIRelayConfig.InsecureSkipVerify,
			},
		},
	}

	logger.Log().Debugf("[apirelay] HTTP client created with timeout: %v, insecure: %v", timeout, config.APIRelayConfig.InsecureSkipVerify)

	// Log request details
	reqDump, _ := httputil.DumpRequestOut(req, true)
	logger.Log().Debugf("[apirelay] Full request:\n%s", string(reqDump))

	// Send request with timeout
	startTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logger.Log().Errorf("[apirelay] Error sending request: %v", err)
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Log response details
	respDump, _ := httputil.DumpResponse(resp, true)
	logger.Log().Debugf("[apirelay] Full response:\n%s", string(respDump))

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Log().Errorf("[apirelay] Error reading response body: %v", err)
		return fmt.Errorf("error reading response body: %v", err)
	}

	// Log response details
	logger.Log().Debugf("[apirelay] Response received in %v - Status: %s", time.Since(startTime), resp.Status)
	logger.Log().Debugf("[apirelay] Response headers: %v", resp.Header)
	logger.Log().Debugf("[apirelay] Response body: %s", string(respBody))

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
		logger.Log().Errorf("[apirelay] %s", errMsg)
		return fmt.Errorf(errMsg)
	}

	logger.Log().Infof(
		"[apirelay] Successfully relayed message from %s to %v via %s",
		from,
		to,
		config.APIRelayConfig.Endpoint,
	)

	return nil
}
