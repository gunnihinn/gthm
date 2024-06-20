package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"gthm/db"

	_ "github.com/mattn/go-sqlite3"
)

var (
	//go:embed schema.sql
	sqlSchema string
)

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

type Feed struct {
	Header  template.HTML
	URL     string
	ID      string
	Entries []Entry
	updated time.Time
}

func (f Feed) Updated() string {
	return f.updated.Format(time.RFC3339)
}

type Entry struct {
	Title   string
	ID      string
	URL     string
	updated time.Time
}

func (e Entry) Updated() string {
	return e.updated.Format(time.RFC3339)
}

type Post struct {
	Id         int64
	Created    time.Time
	Title      string
	Paragraphs []string
}

func (p Post) Date() string {
	return p.Created.Format("2/1/2006")
}

func FromDbPost(p db.Post) Post {
	post := Post{
		Id:      p.ID,
		Created: time.Unix(p.Created, 0),
		Title:   p.Title,
	}

	for _, paragraph := range strings.Split(p.Body, "\n\n") {
		post.Paragraphs = append(post.Paragraphs, strings.TrimSpace(paragraph))
	}

	return post
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

	blog, err := newBlog(*flags.address, *flags.assets, *flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir(path.Join(*flags.assets, "static")))))

	log.Printf("serving blog: port=%d, database=%s, asset-root=%s", *flags.port, *flags.database, *flags.assets)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
