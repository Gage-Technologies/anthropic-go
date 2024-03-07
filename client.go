package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const (
	defaultBaseURL      = "https://api.anthropic.com"
	defaultTimeout      = 600 * time.Second
	defaultMaxRetries   = 2
	defaultUserAgent    = "anthropic-go/0.1.0"
	defaultContentType  = "application/json"
	defaultAccept       = "application/json"
	defaultStreamAccept = "text/event-stream"
	defaultAPIVersion   = "2023-06-01"
	defaultBetaVersion  = ""
)

type Client struct {
	apiKey       string
	authToken    string
	baseURL      string
	httpClient   *http.Client
	maxRetries   int
	userAgent    string
	timeout      time.Duration
	streamAccept string
	apiVersion   string
	betaVersion  string
}

type ClientOption func(*Client)

func WithAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

func WithAuthToken(authToken string) ClientOption {
	return func(c *Client) {
		c.authToken = authToken
	}
}

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
	}
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = timeout
	}
}

func WithStreamAccept(streamAccept string) ClientOption {
	return func(c *Client) {
		c.streamAccept = streamAccept
	}
}

func WithApiVersion(version string) ClientOption {
	return func(c *Client) {
		c.apiVersion = version
	}
}

func WithBetaVersion(version string) ClientOption {
	return func(c *Client) {
		c.betaVersion = version
	}
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL:      defaultBaseURL,
		httpClient:   http.DefaultClient,
		maxRetries:   defaultMaxRetries,
		userAgent:    defaultUserAgent,
		timeout:      defaultTimeout,
		streamAccept: defaultStreamAccept,
		apiVersion:   defaultAPIVersion,
		betaVersion:  defaultBetaVersion,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.apiKey == "" {
		c.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if c.authToken == "" {
		c.authToken = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}

	return c
}

func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, jsonBody(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", defaultContentType)
	req.Header.Set("Accept", defaultAccept)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("anthropic-version", c.apiVersion)
	if c.betaVersion != "" {
		req.Header.Set("anthropic-beta", c.apiVersion)
	}

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	} else if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}

	return req, nil
}

func (c *Client) do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: %s - %s", resp.Status, string(bodyBytes))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func jsonBody(v interface{}) io.Reader {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		panic(err)
	}
	return &buf
}

func idempotencyKey() string {
	return fmt.Sprintf("anthropic-go-retry-%s", uuid.New().String())
}
