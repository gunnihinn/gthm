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
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type blog struct {
	index *template.Template
	db    *sql.DB
}

func newBlog(filename string, database string) (blog, error) {
	blob, err := os.ReadFile(filename)
	if err != nil {
		return blog{}, fmt.Errorf("error: Couldn't read %s: %s", filename, err)
	}
	tmpl, err := template.New("index").Parse(string(blob))
	if err != nil {
		return blog{}, fmt.Errorf("error: Couldn't parse %s template: %s", filename, err)
	}

	db, err := sql.Open("sqlite3", database)
	if err != nil {
		return blog{}, fmt.Errorf("error: Couldn't open database %s: %s", database, err)
	}

	if _, err := db.Exec(schema); err != nil {
		return blog{}, fmt.Errorf("error: Couldn't initialize database schemas: %s", err)
	}

	return blog{index: tmpl, db: db}, nil
}

func (b blog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: url=%s", r.URL)

	rows, err := b.db.QueryContext(context.Background(), "SELECT id, created, title, body FROM posts ORDER BY id DESC")
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

	blog, err := newBlog("index.html", *flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("serving blog: port=%d, database=%s", *flags.port, *flags.database)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
