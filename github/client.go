package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/mchipperfield/blog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "github.com/mchipperfield/blog/github"

type Client struct {
	client *http.Client
	token  string // Github API token used for authentication
}

// NewClient creates a new Client to make requests to the Github API with a default
// HTTP client configured with OpenTelemetry instrumentation and a 10-second timeout.
// Can be configured with ClientOption functions.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("github: client option: %w", err)
		}
	}
	return c, nil
}

// ClientOption configures a Client during construction.
type ClientOption func(*Client) error

// WithToken configures the GitHub token used to authenticate API requests.
// If set, the token is automatically sent in the Authorization header of each API request.
func WithToken(token string) ClientOption {
	return func(c *Client) error {
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		c.token = token
		return nil
	}
}

// GetFile returns a repository-relative file or directory listing from the
// GitHub Contents API.
func (c *Client) GetFile(ctx context.Context, owner, repo, filePath string) ([]byte, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "github.getfile")
	defer span.End()
	span.SetAttributes(
		attribute.String("github.owner", owner),
		attribute.String("github.repo", repo),
		attribute.String("github.path", filePath),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(filePath)), nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create http request failed")
		return nil, fmt.Errorf("github: create http request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.raw")

	resp, err := c.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "http request failed")
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("github: http request canceled: %w", err)
		}
		return nil, fmt.Errorf("github: http request: %w", errors.Join(blog.ErrServiceUnavailable, err))
	}
	defer resp.Body.Close()
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))

	if resp.StatusCode >= http.StatusInternalServerError {
		err := fmt.Errorf("server error: %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("github: %w, %w", blog.ErrServiceUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			span.SetStatus(codes.Error, "file not found")
			return nil, blog.ErrArticleNotFound
		case http.StatusUnauthorized, http.StatusForbidden:
			span.SetStatus(codes.Error, "access denied")
			return nil, blog.ErrAccessDenied
		case http.StatusTooManyRequests:
			span.SetStatus(codes.Error, "rate limited")
			return nil, errors.Join(blog.ErrServiceUnavailable, fmt.Errorf("rate limited"))
		default:
			err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("github: %w", err)
		}
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "read body failed")
		return nil, fmt.Errorf("github: read response body: %w", err)
	}
	span.SetAttributes(attribute.Int("github.response.size", len(content)))
	return content, nil
}
