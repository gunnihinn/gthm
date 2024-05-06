package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	//go:embed index.html
	index string
	//go:embed post.html
	post string
	//go:embed 404.html
	notFound string
	//go:embed schema.sql
	schema string
	//go:embed static/style.css
	//go:embed static/favicon.svg
	fs embed.FS
)

const (
	sqlAllPosts = "SELECT id, created, title, body FROM posts ORDER BY id DESC"
	sqlOnePost  = "SELECT id, created, title, body FROM posts WHERE id = ?"
)

var (
	reRoot = regexp.MustCompile(`^/$`)
	reId   = regexp.MustCompile(`^/([0-9]+)/?$`)
)

type blog struct {
	sync.Mutex
	index    *template.Template
	post     *template.Template
	notFound *template.Template
	db       *sql.DB
}

func (b *blog) addPost(ctx context.Context, title string, body string) error {
	b.Lock()
	defer b.Unlock()
	if _, err := b.db.ExecContext(ctx, "INSERT INTO posts(title, body) VALUES(?, ?)", title, body); err != nil {
		return fmt.Errorf("error: Couldn't insert new post: %s", err)
	}

	return nil
}

func (b *blog) getPosts(ctx context.Context, ids []int) ([]Post, error) {
	b.Lock()
	defer b.Unlock()

	rows, err := func(ids []int) (*sql.Rows, error) {
		if len(ids) > 0 {
			return b.db.QueryContext(ctx, sqlOnePost, ids[0])
		} else {
			return b.db.QueryContext(ctx, sqlAllPosts)
		}
	}(ids)
	if err != nil {

		return nil, fmt.Errorf("error: Couldn't get posts from database: %s", err)
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		var post Post
		var timestamp int64
		var body string
		if err := rows.Scan(&post.Id, &timestamp, &post.Title, &body); err != nil {
			return nil, fmt.Errorf("error: Couldn't get post data from database: %s", err)
		}
		post.Created = time.Unix(timestamp, 0)
		for _, paragraph := range strings.Split(body, "\n\n") {
			post.Paragraphs = append(post.Paragraphs, strings.TrimSpace(paragraph))
		}
		posts = append(posts, post)
	}

	return posts, nil
}

func readTemplate(content string, name string) (*template.Template, error) {
	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return nil, fmt.Errorf("error: Couldn't parse template %s: %s", name, err)
	}

	return tmpl, nil
}

func newBlog(database string) (*blog, error) {
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

	blog.notFound, err = readTemplate(notFound, "notFound")
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

func (b *blog) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	var buf bytes.Buffer
	if err := b.notFound.Execute(&buf, nil); err != nil {
		log.Printf("error: Couldn't generate HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't generate HTML"), http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("error: Couldn't write HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write HTML"), http.StatusNotFound)
		return
	}
}

func (b *blog) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	var ids []int
	if reRoot.MatchString(r.URL.Path) {
		// do nothing
	} else if strId := reId.FindStringSubmatch(r.URL.Path); len(strId) == 2 {
		id, err := strconv.Atoi(strId[1])
		if err != nil {
			log.Printf("error: Couldn't parse integer from %s: %s", strId[1], err)
			b.handleNotFound(w, r)
			return
		}
		ids = append(ids, id)
	} else {
		log.Printf("error: Unknown URL %s", r.URL)
		b.handleNotFound(w, r)
		return
	}

	posts, err := b.getPosts(ctx, ids)
	if err != nil {
		log.Print(err)
		http.Error(w, "Couldn't read posts", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	data := struct{ Posts []Post }{Posts: posts}
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

	blog, err := newBlog(*flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.FileServer(http.FS(fs)))

	log.Printf("serving blog: port=%d, database=%s", *flags.port, *flags.database)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
