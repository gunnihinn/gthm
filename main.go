package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path"

	"gthm/pkg/blog"
)

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

	handler, err := blog.New(*flags.address, *flags.assets, *flags.database)
	if err != nil {
		log.Fatalf("error: Couldn't create blog: %s", err)
	}

	http.Handle("/", handler)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir(path.Join(*flags.assets, "static")))))

	log.Printf("serving blog: port=%d, database=%s, asset-root=%s", *flags.port, *flags.database, *flags.assets)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flags.port), nil))
}
