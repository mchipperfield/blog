package github

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/mchipperfield/blog"
	"github.com/yuin/goldmark"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "github.com/mchipperfield/blog/github"

func NewService(user, repo string) *Service {
	return &Service{
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
		user:      user,
		repo:      repo,
		converter: goldmark.New(),
	}
}

type Service struct {
	client    *http.Client
	user      string
	repo      string
	token     string
	converter goldmark.Markdown
}

func (s *Service) GetArticleBySlug(ctx context.Context, slug string) (*blog.FrontMatter, []byte, error) {
	tracer := otel.Tracer(tracerName)
	tracerCtx, span := tracer.Start(ctx, "github.Service.GetArticleBySlug")
	defer span.End()
	span.SetAttributes(
		attribute.String("github.slug", slug),
		attribute.String("github.user", s.user),
		attribute.String("github.repo", s.repo),
	)

	req, err := http.NewRequestWithContext(tracerCtx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", s.user, s.repo, slug), nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create http request failed")
		return nil, nil, fmt.Errorf("github: create http request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "http request failed")
		return nil, nil, fmt.Errorf("github: http request: %w", err)
	}
	defer resp.Body.Close()
	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("github.rate_limit", resp.Header.Get("RateLimit")),
	)

	if resp.StatusCode == http.StatusNotFound {
		span.SetStatus(codes.Error, "article not found")
		return nil, nil, blog.ErrArticleNotFound
	}
	if resp.StatusCode != http.StatusOK {
		span.RecordError(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
		span.SetStatus(codes.Error, fmt.Sprintf("unexpected status code: %d", resp.StatusCode))
		return nil, nil, fmt.Errorf("github: unexpected status code: %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "read body failed")
		return nil, nil, fmt.Errorf("github: read response body: %w", err)
	}

	var fm blog.FrontMatter
	content, err := frontmatter.Parse(&buf, &fm)
	if err != nil {
		return nil, nil, fmt.Errorf("github: parse frontmatter: %w", err)
	}

	buf.Reset()
	if err := s.converter.Convert(content, &buf); err != nil {
		return nil, nil, fmt.Errorf("github: convert markdown: %w", err)
	}

	return &fm, buf.Bytes(), nil
}
