package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type blog struct {
	sync.Mutex
	index *template.Template
	post  *template.Template
	db    *sql.DB
}

func (b *blog) addPost(ctx context.Context, title string, body string) error {
	b.Lock()
	defer b.Unlock()
	if _, err := b.db.ExecContext(ctx, "INSERT INTO posts(title, body) VALUES(?, ?)", title, body); err != nil {
		return fmt.Errorf("error: Couldn't insert new post: %s", err)
	}

	return nil
}

func readTemplate(filename string, name string) (*template.Template, error) {
	blob, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error: Couldn't read %s: %s", filename, err)
	}
	tmpl, err := template.New(name).Parse(string(blob))
	if err != nil {
		return nil, fmt.Errorf("error: Couldn't parse %s template: %s", filename, err)
	}

	return tmpl, nil
}

func newBlog(index string, post string, database string) (*blog, error) {
	blog := &blog{}
	var err error

	blog.index, err = readTemplate(index, "index")
	if err != nil {
		return blog, err
	}

	blog.post, err = readTemplate(post, "post")
	if err != nil {
		return blog, err
	}

	blog.db, err = sql.Open("sqlite3", database)
	if err != nil {
		return blog, fmt.Errorf("error: Couldn't open database %s: %s", database, err)
	}

	if _, err := blog.db.Exec(schema); err != nil {
		return blog, fmt.Errorf("error: Couldn't initialize database schemas: %s", err)
	}

	return blog, nil
}

func (b *blog) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	b.Lock()
	defer b.Unlock()

	rows, err := b.db.QueryContext(ctx, "SELECT id, created, title, body FROM posts ORDER BY id DESC")
	if err != nil {
		log.Printf("error: Couldn't get posts from database: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't read posts"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := struct {
		Posts []Post
	}{
		Posts: make([]Post, 0),
	}
	for rows.Next() {
		var post Post
		var timestamp int64
		var body string
		if err := rows.Scan(&post.Id, &timestamp, &post.Title, &body); err != nil {
			log.Printf("error: Couldn't get post data from database: %s", err)
			http.Error(w, fmt.Sprintf("Couldn't read post"), http.StatusInternalServerError)
			return
		}
		post.Created = time.Unix(timestamp, 0)
		for _, paragraph := range strings.Split(body, "\n\n") {
			post.Paragraphs = append(post.Paragraphs, strings.TrimSpace(paragraph))
		}
		data.Posts = append(data.Posts, post)
	}

	var buf bytes.Buffer
	if err := b.index.Execute(&buf, data); err != nil {
		log.Printf("error: Couldn't generate HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't generate HTML"), http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("error: Couldn't write HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write HTML"), http.StatusInternalServerError)
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

		titles, ok := r.Form["title"]
		if !ok {
			log.Printf("error: No title in form")
			http.Error(w, fmt.Sprintf("No title in form"), http.StatusInternalServerError)
			return
		}
		title := strings.TrimSpace(strings.Join(titles, " "))

		bodies, ok := r.Form["body"]
		if !ok || len(bodies) == 0 {
			log.Printf("error: No body in form")
			http.Error(w, fmt.Sprintf("No body in form"), http.StatusInternalServerError)
			return
		}
		body := strings.TrimSpace(strings.ReplaceAll(strings.Join(bodies, "\n\n"), "\r\n", "\n"))

		if err := b.addPost(ctx, title, body); err != nil {
			log.Print(err)
			http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
			return
		}

		b.handleIndex(w, r)
		return
	}

	var buf bytes.Buffer
	if err := b.post.Execute(&buf, nil); err != nil {
		log.Printf("error: Couldn't generate HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't generate HTML"), http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("error: Couldn't write HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write HTML"), http.StatusInternalServerError)
		return
	}

}

func (b *blog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: url=%s", r.URL)

	if r.URL.Path == "/new" {
		b.handleWrite(w, r)
	} else {
		b.handleIndex(w, r)
	}
}

type Post struct {
	Id         int
	Created    time.Time
	Title      string
	Paragraphs []string
}

func (p Post) Date() string {
	return p.Created.Format("2/1/2006")
}

func main() {
	flags := struct {
		port     *int
		database *string
	}{
		port:     flag.Int("port", 8000, "port to serve blog on"),
		database: flag.String("database", ":memory:", "database to store posts in"),
	}
	flag.Parse()

	blog, err := newBlog("index.html", "post.html", *flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("serving blog: port=%d, database=%s", *flags.port, *flags.database)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
