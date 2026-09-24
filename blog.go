// Package blog retrieves Markdown articles, extracts their frontmatter, renders
// their content as HTML, and provides an optional net/http presentation layer.
package blog

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrArticleNotFound indicates that no article exists for the requested slug.
	ErrArticleNotFound = errors.New("blog: article not found")
	// ErrServiceUnavailable indicates that a required backing service could not
	// complete the operation.
	ErrServiceUnavailable = errors.New("blog: service unavailable")
	// ErrAccessDenied indicates lack of permission to access the requested resource. e.g. no read access.
	ErrAccessDenied = errors.New("blog: access denied")
	// ErrInvalidSlug indicates that the provided slug is not valid.
	ErrInvalidSlug = errors.New("blog: invalid slug")
)

// Service provides rendered articles and article metadata independently of the
// backing store and HTTP representation.
type Service interface {
	// GetArticleBySlug returns an article's frontmatter and rendered HTML content.
	GetArticleBySlug(ctx context.Context, slug string) (*Metadata, []byte, error)
	// ListArticles returns metadata for the articles that could be loaded.
	ListArticles(ctx context.Context) ([]*Metadata, error)
}

// Store retrieves raw Markdown from a backing store.
type Store interface {
	// GetArticleBySlug returns an Article containing the frontmatter and raw Markdown content for the given slug.
	GetArticleBySlug(ctx context.Context, slug string) (*Article, error)
	// ListArticles returns article metadata for all available articles.
	ListArticles(ctx context.Context) ([]*Metadata, error)
}

// Metadata contains the metadata extracted from an article document.
type Metadata struct {
	Title       string
	Slug        string
	Description string
	Tags        []string
	Author      string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

type Article struct {
	Metadata *Metadata
	Markdown []byte
}
