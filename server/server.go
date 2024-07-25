package server

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
)

//go:embed dist
var public embed.FS

var jsonFile []byte

func setupHandlers() {
	http.HandleFunc("/wally.json", jsonHandler)

	publicFS, err := fs.Sub(public, "dist")
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", http.FileServer(http.FS(publicFS)))
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(jsonFile)
}

func ServerCosmograph(file []byte, port int) {
	jsonFile = file

	setupHandlers()

	if port == 0 {
		port = 1984
	}
	portStr := strconv.Itoa(port)

	fmt.Printf("Wally server running on http://localhost:%s", portStr)

	err := http.ListenAndServe(fmt.Sprintf(":%s", portStr), nil)
	if err != nil {
		log.Fatal("Unable to start server with err: ", err)
	}
}
