package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

type blog struct {
	index *template.Template
}

func newBlog(filename string) (blog, error) {
	blob, err := os.ReadFile(filename)
	if err != nil {
		return blog{}, fmt.Errorf("error: Couldn't read %s: %s", filename, err)
	}
	tmpl, err := template.New("index").Parse(string(blob))
	if err != nil {
		return blog{}, fmt.Errorf("error: Couldn't parse %s template: %s", filename, err)
	}

	return blog{index: tmpl}, nil
}

func (b blog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("postHandler: url=%s", r.URL)

	var buf bytes.Buffer
	if err := b.index.Execute(&buf, nil); err != nil {
		log.Printf("error: Couldn't generate HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't serve HTML"), http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("error: Couldn't write HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't serve HTML"), http.StatusInternalServerError)
		return
	}
}

func main() {
	flags := struct {
		port *int
	}{
		port: flag.Int("port", 8000, "port to serve blog on"),
	}
	flag.Parse()

	blog, err := newBlog("index.html")
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", blog)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("serving blog: port=%d", *flags.port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
