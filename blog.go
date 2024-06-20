package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"gthm/db"
)

var (
	reRoot = regexp.MustCompile(`^/$`)
	reId   = regexp.MustCompile(`^/([0-9]+)/?$`)
)

type blog struct {
	sync.Mutex
	address  string
	index    *template.Template
	post     *template.Template
	notFound *template.Template
	atom     *template.Template
	db       *sql.DB
}

func newBlog(address string, assets string, database string) (*blog, error) {
	blog := &blog{address: address}
	var err error

	blog.index, err = readTemplate(assets, "index.html", "index")
	if err != nil {
		return blog, err
	}

	blog.post, err = readTemplate(assets, "post.html", "post")
	if err != nil {
		return blog, err
	}

	blog.notFound, err = readTemplate(assets, "404.html", "notFound")
	if err != nil {
		return blog, err
	}

	blog.atom, err = readTemplate(assets, "feed.xml", "feed")
	if err != nil {
		return blog, err
	}

	blog.db, err = sql.Open("sqlite3", fmt.Sprintf("%s?_busy_timeout=500&_journal_mode=WAL", database))
	if err != nil {
		return blog, fmt.Errorf("error: Couldn't open database %s: %s", database, err)
	}

	if _, err := blog.db.Exec(sqlSchema); err != nil {
		return blog, fmt.Errorf("error: Couldn't initialize database schemas: %s", err)
	}

	return blog, nil
}

func (b *blog) addPost(ctx context.Context, title string, body string) error {
	b.Lock()
	defer b.Unlock()

	if err := db.New(b.db).CreatePost(ctx, db.CreatePostParams{Title: title, Body: body}); err != nil {
		return fmt.Errorf("error: Couldn't insert new post: %s", err)
	}

	return nil
}

func (b *blog) handleFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	b.Lock()
	defer b.Unlock()

	ps, err := db.New(b.db).ListPosts(ctx)
	if err != nil {
		log.Printf("error: %s", err)
		http.Error(w, "Couldn't read posts", http.StatusInternalServerError)
		return
	}

	posts := make([]Post, 0, len(ps))
	for _, p := range ps {
		posts = append(posts, FromDbPost(p))
	}

	feed := Feed{
		ID:     b.address,
		URL:    fmt.Sprintf("%s/feed", b.address),
		Header: template.HTML(`<?xml version="1.0" encoding="utf-8"?>`),
	}
	if len(posts) > 0 {
		feed.updated = posts[0].Created
	} else {
		feed.updated = time.Now()
	}
	for _, post := range posts {
		e := Entry{
			Title:   post.Title,
			ID:      fmt.Sprintf("%s/%d", b.address, post.Id),
			URL:     fmt.Sprintf("%s/%d", b.address, post.Id),
			updated: post.Created,
		}
		feed.Entries = append(feed.Entries, e)
	}

	if err := writeTemplate(w, b.atom, feed); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *blog) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	if err := writeTemplate(w, b.notFound, nil); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *blog) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	b.Lock()
	defer b.Unlock()

	ps, err := db.New(b.db).ListPosts(ctx)
	if err != nil {
		log.Printf("error: %s", err)
		http.Error(w, "Couldn't read posts", http.StatusInternalServerError)
		return
	}

	posts := make([]Post, 0, len(ps))
	for _, p := range ps {
		posts = append(posts, FromDbPost(p))
	}

	data := struct{ Posts []Post }{Posts: posts}
	if err := writeTemplate(w, b.index, data); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func getID(path string) (int, error) {
	strId := reId.FindStringSubmatch(path)
	if len(strId) <= 1 {
		return 0, fmt.Errorf("Expected post ID")
	}

	return strconv.Atoi(strId[1])
}

func (b *blog) handleOnePost(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	id, err := getID(r.URL.Path)
	if err != nil {
		log.Printf("error: %s", err)
		b.handleNotFound(w, r)
		return
	}

	b.Lock()
	defer b.Unlock()

	query := db.New(b.db)
	post, err := query.GetPost(ctx, int64(id))
	if err != nil {
		log.Printf("error: %s", err)
		http.Error(w, "Couldn't read post", http.StatusInternalServerError)
		return
	}

	posts := []Post{FromDbPost(post)}
	data := struct{ Posts []Post }{Posts: posts}
	if err := writeTemplate(w, b.index, data); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *blog) handleWrite(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			log.Printf("error: Couldn't parse HTML form: %s", err)
			http.Error(w, fmt.Sprintf("Couldn't parse HTML form: %s", err), http.StatusInternalServerError)
			return
		}

		title, body, err := parseForm(r.Form)
		if err != nil {
			log.Printf("error: %s", err)
			http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
			return
		}

		b.Lock()
		defer b.Unlock()
		if err := db.New(b.db).CreatePost(ctx, db.CreatePostParams{Title: title, Body: body}); err != nil {
			log.Print(err)
			http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, b.address, http.StatusSeeOther)
		return
	}

	if err := writeTemplate(w, b.post, nil); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *blog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: url=%s", r.URL)

	if r.URL.Path == "/new" {
		b.handleWrite(w, r)
	} else if r.URL.Path == "/feed" || r.URL.Path == "/feed/" {
		b.handleFeed(w, r)
	} else if reRoot.MatchString(r.URL.Path) {
		b.handleIndex(w, r)
	} else if reId.MatchString(r.URL.Path) {
		b.handleOnePost(w, r)
	} else {
		b.handleNotFound(w, r)
	}
}
