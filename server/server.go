package server

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed dist
var public embed.FS

var jsonFile []byte

func setupHandlers() {
	http.HandleFunc("/wally.json", jsonHandler)
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(jsonFile)
	return
}

func ServerCosmograph(file []byte) {
	jsonFile = file

	publicFS, err := fs.Sub(public, "dist")
	if err != nil {
		log.Fatal(err)
	}

	setupHandlers()
	http.Handle("/", http.FileServer(http.FS(publicFS)))

	port := ":9999"
	log.Fatal(http.ListenAndServe(port, nil))
}
