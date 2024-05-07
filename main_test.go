package main

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"
)

func TestGetIds(t *testing.T) {
	type setup struct {
		path   string
		expIds []int
		expErr error
	}
	setups := []setup{
		{path: "/"},
		{path: "/1", expIds: []int{1}},
		{path: "/100", expIds: []int{100}},
		{path: "/1000000000000000000000000000000000000000000", expErr: fmt.Errorf("strconv.Atoi error")},
		{path: "/foo", expErr: fmt.Errorf("Some error")},
	}

	for _, s := range setups {
		u := &url.URL{Path: s.path}

		ids, err := getIds(u)
		if err != nil && s.expErr == nil {
			t.Errorf("expected no error, got %s", err)
		} else if err == nil && s.expErr != nil {
			t.Errorf("expected error %s, got nothing", s.expErr)
		}

		if len(ids) != len(s.expIds) {
			t.Errorf("expected %d IDs, got %d IDs", len(s.expIds), len(ids))
		}

		for i, id := range ids {
			if s.expIds[i] != id {
				t.Errorf("expected %d, got %d", s.expIds[i], id)
			}
		}
	}
}

func TestReadTemplate(t *testing.T) {
	names := []string{"index.html", "post.html", "404.html"}
	for _, name := range names {
		tmpl, err := readTemplate("assets", name, name)
		if err != nil {
			t.Errorf("expected no error, got %s", err)
		}

		if tmpl == nil {
			t.Errorf("expected non-nil template")
		}
	}
}

func TestBlog(t *testing.T) {
	blog, err := newBlog("assets", ":memory:")
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	posts, err := blog.getPosts(ctx, nil)
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}
	if len(posts) > 0 {
		t.Errorf("expected no posts, got %d", len(posts))
	}

	if err := blog.addPost(ctx, "1", "body"); err != nil {
		t.Errorf("expected no error, got %s", err)
	}

	if err := blog.addPost(ctx, "2", "body"); err != nil {
		t.Errorf("expected no error, got %s", err)
	}

	posts, err = blog.getPosts(ctx, nil)
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}
	if len(posts) != 2 {
		t.Errorf("expected 2 posts, got %d", len(posts))
	}

	posts, err = blog.getPosts(ctx, []int{2})
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}
	if len(posts) != 1 {
		t.Errorf("expected 1 post, got %d", len(posts))
	}
	if posts[0].Title != "2" {
		t.Errorf("expected post titled 2, got %s", posts[0].Title)
	}
}
