package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func postHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("postHandler: url=%s", r.URL)
	blob, err := os.ReadFile("index.html")
	if err != nil {
		log.Printf("error: Couldn't read HTML: %s", err)
		http.Error(w, fmt.Sprintf("Couldn't serve HTML"), http.StatusInternalServerError)
		return
	}

	w.Write(blob)
}

func main() {
	http.HandleFunc("/", postHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Fatal(http.ListenAndServe(":8000", nil))
}
