// Package github provides a GitHub Contents API implementation of blog.Store for a specific owner and repository.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/mchipperfield/blog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Service retrieves Markdown articles through the GitHub Contents API using the provided HTTP client.
type Service struct {
	client      *Client
	owner       string
	repo        string
	articlePath string
}

// NewService creates a GitHub-backed article store for owner and repo.
func NewService(client *Client, owner, repo string, opts ...ServiceOption) (*Service, error) {
	if owner == "" {
		return nil, errors.New("github: owner cannot be empty")
	}
	if repo == "" {
		return nil, errors.New("github: repo cannot be empty")
	}
	if client == nil {
		return nil, errors.New("github: client cannot be nil")
	}
	svc := &Service{
		client: client,
		owner:  owner,
		repo:   repo,
	}
	for _, opt := range opts {
		if err := opt(svc); err != nil {
			return nil, fmt.Errorf("github: service init: %w", err)
		}
	}
	return svc, nil
}

// ServiceOption configures a Service during construction.
type ServiceOption func(*Service) error

// WithArticlePath configures the repository-relative directory containing
// article files. An empty path explicitly selects the repository root.
func WithArticlePath(articlePath string) ServiceOption {
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

// GetArticleBySlug returns the raw Markdown file named <slug>.md from the
// configured article directory.
func (s *Service) GetArticleBySlug(ctx context.Context, slug string) (*blog.Article, error) {
	tracer := otel.Tracer(tracerName)
	tracerCtx, span := tracer.Start(ctx, "article.getbyslug")
	defer span.End()
	span.SetAttributes(
		attribute.String("article.slug", slug),
		attribute.String("article.backend", "github"),
		attribute.String("github.owner", s.owner),
		attribute.String("github.repo", s.repo),
	)
	if !blog.ValidSlug(slug) {
		return nil, blog.ErrInvalidSlug
	}
	articlePath := path.Join(s.articlePath, slug+".md")
	markdown, err := s.client.GetFile(tracerCtx, s.owner, s.repo, articlePath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get file failed")
		return nil, fmt.Errorf("github: get file: %w", err)
	}

	var fm blog.Metadata
	content, err := frontmatter.Parse(bytes.NewReader(markdown), &fm)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "parse frontmatter failed")
		return nil, fmt.Errorf("github: parse frontmatter: %w", err)
	}

	return &blog.Article{
		Metadata: &fm,
		Markdown: content,
	}, nil
}

// ListArticles returns Markdown filenames from the configured article
// directory. Subdirectories are not traversed.
func (s *Service) ListArticles(ctx context.Context) ([]*blog.Metadata, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "article.list")
	span.SetAttributes(
		attribute.String("article.backend", "github"),
		attribute.String("github.owner", s.owner),
		attribute.String("github.repo", s.repo),
	)
	defer span.End()

	content, err := s.client.GetFile(ctx, s.owner, s.repo, s.articlePath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list directory failed")
		return nil, fmt.Errorf("github: list directory: %w", err)
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(content, &entries); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "decode directory failed")
		return nil, fmt.Errorf("github: decode directory: %w", err)
	}

	// 2. Keep only Markdown files; nested directories are deliberately ignored.
	var articles []*blog.Metadata
	for _, e := range entries {
		if e.Type == "file" && strings.HasSuffix(e.Name, ".md") {
			article, err := s.client.GetFile(ctx, s.owner, s.repo, e.Path)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, fmt.Errorf("github: list articles: %w", err)
				}
				continue
			}
			var fm blog.Metadata
			_, err = frontmatter.Parse(bytes.NewReader(article), &fm)
			if err != nil {
				continue
			}
			fm.Slug = strings.TrimSuffix(e.Name, ".md")
			articles = append(articles, &fm)

		}
	}
	span.SetAttributes(attribute.Int("article.count", len(articles)))
	return articles, nil
}
