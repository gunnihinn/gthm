package blog

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gthm/pkg/atom"
	"gthm/pkg/db"

	_ "github.com/mattn/go-sqlite3"
)

var (
	reRoot = regexp.MustCompile(`^/$`)
	reId   = regexp.MustCompile(`^/([0-9]+)/?$`)
	//go:embed schema.sql
	sqlSchema string
)

type Post struct {
	db.Post
	Paragraphs []string
}

func (p Post) Date() string {
	return time.Unix(p.Created, 0).Format("2/1/2006")
}

func FromDbPost(p db.Post) Post {
	post := Post{Post: p}

	for _, paragraph := range strings.Split(p.Body, "\n\n") {
		post.Paragraphs = append(post.Paragraphs, strings.TrimSpace(paragraph))
	}

	return post
}

type Blog struct {
	sync.Mutex
	address  string
	index    *template.Template
	post     *template.Template
	notFound *template.Template
	atom     *template.Template
	db       *sql.DB
}

func New(address string, assets string, database string) (*Blog, error) {
	blog := &Blog{address: address}
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

func (b *Blog) addPost(ctx context.Context, title string, body string) error {
	b.Lock()
	defer b.Unlock()

	if err := db.New(b.db).CreatePost(ctx, db.CreatePostParams{Title: title, Body: body}); err != nil {
		return fmt.Errorf("error: Couldn't insert new post: %s", err)
	}

	return nil
}

func (b *Blog) handleFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	b.Lock()
	defer b.Unlock()

	posts, err := db.New(b.db).ListPosts(ctx)
	if err != nil {
		log.Printf("error: %s", err)
		http.Error(w, "Couldn't read posts", http.StatusInternalServerError)
		return
	}

	feed := atom.FromPosts(posts, b.address)
	if err := writeTemplate(w, b.atom, feed); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *Blog) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	if err := writeTemplate(w, b.notFound, nil); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't write response"), http.StatusInternalServerError)
		return
	}
}

func (b *Blog) handleIndex(w http.ResponseWriter, r *http.Request) {
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

func (b *Blog) handleOnePost(w http.ResponseWriter, r *http.Request) {
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

func (b *Blog) handleWrite(w http.ResponseWriter, r *http.Request) {
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

func (b *Blog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func parseForm(form map[string][]string) (string, string, error) {
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

func writeTemplate(w io.Writer, tmpl *template.Template, data any) error {
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return fmt.Errorf("Couldn't execute template: %s", err)
	}

	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("Couldn't write template: %s", err)
	}

	return nil
}
