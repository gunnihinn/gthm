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
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	//go:embed schema.sql
	schema string
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
	address  string
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

		return nil, fmt.Errorf("Couldn't get posts from database: %s", err)
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		var post Post
		var timestamp int64
		var body string
		if err := rows.Scan(&post.Id, &timestamp, &post.Title, &body); err != nil {
			return nil, fmt.Errorf("Couldn't get post data from database: %s", err)
		}
		post.Created = time.Unix(timestamp, 0)
		for _, paragraph := range strings.Split(body, "\n\n") {
			post.Paragraphs = append(post.Paragraphs, strings.TrimSpace(paragraph))
		}
		posts = append(posts, post)
	}

	return posts, nil
}

func readTemplate(assets string, filename string, name string) (*template.Template, error) {
	fn := path.Join(assets, filename)
	blob, err := os.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("error: Couldn't read %s: %s", fn, err)
	}
	tmpl, err := template.New(name).Parse(string(blob))
	if err != nil {
		return nil, fmt.Errorf("error: Couldn't parse template %s: %s", name, err)
	}

	return tmpl, nil
}

func newBlog(assets string, database string) (*blog, error) {
	blog := &blog{}
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

	blog.db, err = sql.Open("sqlite3", fmt.Sprintf("%s?_busy_timeout=500&_journal_mode=WAL", database))
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

func getIds(u *url.URL) ([]int, error) {
	var ids []int
	if reRoot.MatchString(u.Path) {
		return ids, nil
	} else if strId := reId.FindStringSubmatch(u.Path); len(strId) == 2 {
		id, err := strconv.Atoi(strId[1])
		if err != nil {
			return nil, fmt.Errorf("Couldn't parse integer from %s: %s", strId[1], err)
		}
		ids = append(ids, id)
	} else {
		return nil, fmt.Errorf("Unknown URL %s", u)
	}

	return ids, nil
}

func (b *blog) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	ids, err := getIds(r.URL)
	if err != nil {
		log.Printf("error: %s", err)
		b.handleNotFound(w, r)
		return
	}

	posts, err := b.getPosts(ctx, ids)
	if err != nil {
		log.Printf("error: %s", err)
		http.Error(w, "Couldn't read posts", http.StatusInternalServerError)
		return
	}

	buf := new(bytes.Buffer)
	data := struct{ Posts []Post }{Posts: posts}
	if err := b.index.Execute(buf, data); err != nil {
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

func parsePost(form map[string][]string) (string, string, error) {
	titles, ok := form["title"]
	if !ok {
		return "", "", fmt.Errorf("No title in form")
	}
	title := strings.TrimSpace(strings.Join(titles, " "))

	bodies, ok := form["body"]
	if !ok || len(bodies) == 0 {
		return "", "", fmt.Errorf("No body in form")
	}
	body := strings.TrimSpace(strings.ReplaceAll(strings.Join(bodies, "\n\n"), "\r\n", "\n"))

	return title, body, nil
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

		title, body, err := parsePost(r.Form)
		if err != nil {
			log.Printf("error: %s", err)
			http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
			return
		}

		if err := b.addPost(ctx, title, body); err != nil {
			log.Print(err)
			http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, b.address, http.StatusSeeOther)
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
		address  *string
		assets   *string
	}{
		port:     flag.Int("port", 8000, "port to serve blog on"),
		database: flag.String("database", ":memory:", "database to store posts in"),
		address:  flag.String("address", "https://www.gthm.is", "public address of blog"),
		assets:   flag.String("assets", "assets", "root directory of assets"),
	}
	flag.Parse()

	blog, err := newBlog(*flags.assets, *flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir(path.Join(*flags.assets, "static")))))

	log.Printf("serving blog: port=%d, database=%s, asset-root=%s", *flags.port, *flags.database, *flags.assets)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
