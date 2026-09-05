// Package github provides a GitHub Contents API implementation of blog.Store.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/mchipperfield/blog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "github.com/mchipperfield/blog/github"

// NewService creates a GitHub-backed article store for user and repo.
func NewService(user, repo string, opts ...Option) (*Service, error) {
	if user == "" {
		return nil, fmt.Errorf("user cannot be empty")
	}
	if repo == "" {
		return nil, fmt.Errorf("repo cannot be empty")
	}
	svc := &Service{
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
		user: user,
		repo: repo,
	}
	for _, opt := range opts {
		if err := opt(svc); err != nil {
			return nil, fmt.Errorf("github: handler init: %w", err)
		}
	}
	return svc, nil
}

// Option configures a Service during construction.
type Option func(*Service) error

// WithToken configures the GitHub token used to authenticate API requests.
func WithToken(token string) Option {
	return func(s *Service) error {
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		s.token = token
		return nil
	}
}

// WithArticlePath configures the repository-relative directory containing
// article files. An empty path explicitly selects the repository root.
func WithArticlePath(articlePath string) Option {
	return func(s *Service) error {
		cleanedPath := path.Clean(articlePath)
		if cleanedPath == "." {
			cleanedPath = ""
		}
		if path.IsAbs(cleanedPath) ||
			cleanedPath == ".." ||
			strings.HasPrefix(cleanedPath, "../") {
			return errors.New("article path must be relative to repository root")
		}
		s.articlePath = cleanedPath
		return nil
	}
}

// Service retrieves Markdown articles through the GitHub Contents API.
type Service struct {
	client      *http.Client
	user        string
	repo        string
	token       string
	articlePath string
}

// GetArticleBySlug returns the raw Markdown file named <slug>.md from the
// configured article directory.
func (s *Service) GetArticleBySlug(ctx context.Context, slug string) ([]byte, error) {
	tracer := otel.Tracer(tracerName)
	tracerCtx, span := tracer.Start(ctx, "article.getbyslug")
	defer span.End()
	span.SetAttributes(
		attribute.String("article.slug", slug),
		attribute.String("article.backend", "github"),
		attribute.String("github.user", s.user),
		attribute.String("github.repo", s.repo),
	)

	articlePath := path.Join(s.articlePath, slug+".md")
	req, err := http.NewRequestWithContext(tracerCtx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(s.user), url.PathEscape(s.repo), url.PathEscape(articlePath)), nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create http request failed")
		return nil, fmt.Errorf("github: create http request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.raw")

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "http request failed")
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("github: http request canceled: %w", err)
		}
		return nil, fmt.Errorf("github: http request: %w", errors.Join(blog.ErrServiceUnavailable, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		span.RecordError(fmt.Errorf("server error: %d", resp.StatusCode))
		span.SetStatus(codes.Error, fmt.Sprintf("server error: %d", resp.StatusCode))
		return nil, fmt.Errorf("github: %w, server error: %d", blog.ErrServiceUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			span.SetStatus(codes.Error, "article not found")
			return nil, blog.ErrArticleNotFound
		case http.StatusUnauthorized:
			span.SetStatus(codes.Error, "unauthorized")
			return nil, fmt.Errorf("github: unauthorized")
		default:
			span.RecordError(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			span.SetStatus(codes.Error, fmt.Sprintf("unexpected status code: %d", resp.StatusCode))
			return nil, fmt.Errorf("github: unexpected status code: %d", resp.StatusCode)
		}
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "read body failed")
		return nil, fmt.Errorf("github: read response body: %w", err)
	}

	return content, nil
}

// ListArticles returns Markdown filenames from the configured article
// directory. Subdirectories are not traversed.
func (s *Service) ListArticles(ctx context.Context) ([]string, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "article.list")
	span.SetAttributes(
		attribute.String("article.backend", "github"),
		attribute.String("github.user", s.user),
		attribute.String("github.repo", s.repo),
	)
	defer span.End()

	// 1. List the configured article directory through the GitHub Contents API.
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(s.user), url.PathEscape(s.repo), url.PathEscape(s.articlePath)),
		nil,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create request failed")
		return nil, fmt.Errorf("github: list directory: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "http request failed")
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("github: http request canceled: %w", err)
		}
		return nil, fmt.Errorf("github: http request: %w", errors.Join(blog.ErrServiceUnavailable, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		span.RecordError(fmt.Errorf("server error: %d", resp.StatusCode))
		span.SetStatus(codes.Error, fmt.Sprintf("server error: %d", resp.StatusCode))
		return nil, fmt.Errorf("github: %w, server error: %d", blog.ErrServiceUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			span.SetStatus(codes.Error, "unauthorized")
			return nil, fmt.Errorf("github: unauthorized")
		default:
			span.RecordError(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			span.SetStatus(codes.Error, fmt.Sprintf("unexpected status code: %d", resp.StatusCode))
			return nil, fmt.Errorf("github: unexpected status code: %d", resp.StatusCode)
		}
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "decode directory failed")
		return nil, fmt.Errorf("github: decode directory: %w", err)
	}

	// 2. Keep only Markdown files; nested directories are deliberately ignored.
	var markdownFiles []string
	for _, e := range entries {
		if e.Type == "file" && strings.HasSuffix(e.Name, ".md") {
			markdownFiles = append(markdownFiles, e.Name)
		}
	}

	span.SetAttributes(attribute.Int("article.count", len(markdownFiles)))
	return markdownFiles, nil
}
