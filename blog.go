// Package blog retrieves Markdown articles, extracts their frontmatter, renders
// their content as HTML, and provides an optional net/http presentation layer.
package blog

import (
	"context"
	"errors"
	"html/template"
	"time"
)

var (
	// ErrArticleNotFound indicates that no article exists for the requested slug.
	ErrArticleNotFound = errors.New("blog: article not found")
	// ErrServiceUnavailable indicates that a required backing service could not
	// complete the operation.
	ErrServiceUnavailable = errors.New("blog: service unavailable")
)

// Service provides rendered articles and article metadata independently of the
// backing store and HTTP representation.
type Service interface {
	// GetArticleBySlug returns an article's frontmatter and rendered HTML content.
	GetArticleBySlug(ctx context.Context, slug string) (*Frontmatter, []byte, error)
	// ListArticles returns metadata for the articles that could be loaded.
	ListArticles(ctx context.Context) ([]*Frontmatter, error)
}

// Store retrieves raw Markdown documents and lists their filenames.
type Store interface {
	// GetArticleBySlug returns the raw Markdown document for slug.
	GetArticleBySlug(ctx context.Context, slug string) ([]byte, error)
	// ListArticles returns article filenames, including their .md extension.
	ListArticles(ctx context.Context) ([]string, error)
}

// Frontmatter contains the metadata extracted from an article document.
type Frontmatter struct {
	Title       string
	Slug        string
	Description string
	Tags        []string
	Author      string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

// ArticleView is the data contract for the "article" HTML template. Content is
// trusted HTML produced by the configured Service.
type ArticleView struct {
	FrontMatter *Frontmatter
	Content     template.HTML
}

// IndexView is the data contract for the "index" HTML template.
type IndexView struct {
	Articles []*Frontmatter
}
