package blog

import (
	"context"
	"errors"
	"html/template"
	"time"
)

var ErrArticleNotFound = errors.New("blog: article not found")
var ErrServiceUnavailable = errors.New("blog: service unavailable")

type Service interface {
	GetArticleBySlug(ctx context.Context, slug string) (*Frontmatter, []byte, error)
	ListArticles(ctx context.Context) ([]*Frontmatter, error)
}

type Store interface {
	GetArticleBySlug(ctx context.Context, slug string) ([]byte, error)
	ListArticles(ctx context.Context) ([]string, error)
}

type Frontmatter struct {
	Title       string
	Slug        string
	Description string
	Tags        []string
	Author      string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

type ArticleView struct {
	FrontMatter *Frontmatter
	Content     template.HTML
}

type IndexView struct {
	Articles []*Frontmatter
}
