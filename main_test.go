package main

import (
	"bytes"
	"context"
	"fmt"
	"gthm/db"
	"testing"
	"time"
)

func TestGetIds(t *testing.T) {
	type setup struct {
		path   string
		expID  int
		expErr error
	}
	setups := []setup{
		{path: "/", expErr: fmt.Errorf("Expected post ID")},
		{path: "/1", expID: 1},
		{path: "/100", expID: 100},
		{path: "/1000000000000000000000000000000000000000000", expErr: fmt.Errorf("strconv.Atoi error")},
		{path: "/foo", expErr: fmt.Errorf("Some error")},
	}

	for _, s := range setups {
		id, err := getID(s.path)
		if err != nil && s.expErr == nil || err == nil && s.expErr != nil {
			t.Errorf("error mismatch: got '%v', expected '%v'", err, s.expErr)
		}

		if err != nil {
			continue
		}

		if s.expID != id {
			t.Errorf("ID mismatch: expected %d, got %d", s.expID, id)
		}
	}
}

func TestReadTemplate(t *testing.T) {
	names := []string{"index.html", "post.html", "404.html"}
	data := struct{ Posts []Post }{Posts: []Post{}}
	for _, name := range names {
		tmpl, err := readTemplate("assets", name, name)
		if err != nil {
			t.Errorf("expected no error, got %s", err)
		}

		if tmpl == nil {
			t.Errorf("expected non-nil template")
		}

		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, data); err != nil {
			t.Errorf("expected no error, got %s", err)
		}
	}
}

func TestBlog(t *testing.T) {
	blog, err := newBlog("none", "assets", ":memory:")
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	query := db.New(blog.db)

	posts, err := query.ListPosts(ctx)
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

	posts, err = query.ListPosts(ctx)
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}
	if len(posts) != 2 {
		t.Errorf("expected 2 posts, got %d", len(posts))
	}

	post, err := query.GetPost(ctx, 2)
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}
	if post.Title != "2" {
		t.Errorf("expected post titled 2, got %s", posts[0].Title)
	}
}

func TestParsePost(t *testing.T) {
	form := make(map[string][]string)

	if _, _, err := parseForm(form); err == nil {
		t.Errorf("expected error")
	}

	form["title"] = []string{"title"}
	if _, _, err := parseForm(form); err == nil {
		t.Errorf("expected error")
	}

	form["body"] = []string{"body\r\nbody"}
	title, body, err := parseForm(form)
	if err != nil {
		t.Errorf("expected no error, got %s", err)
	}

	if title != "title" {
		t.Errorf("expected title, got %s", title)
	}

	if body != "body\nbody" {
		t.Errorf("expected body<nl>body, got\n%s", body)
	}
}
