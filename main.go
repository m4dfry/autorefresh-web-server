package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// local site folder default value
var siteFolder string = "./site"

// websocket endpoint on server default value
var wsAPIURL string = "rws"

// server port default value
var port string = "3000"

var jsws string

// system file watcher
var watcher *fsnotify.Watcher

func httpHandler(w http.ResponseWriter, req *http.Request) {
	// build file path
	path := siteFolder + req.URL.Path

	// fallback to index if root is requested
	if strings.HasSuffix(path, "/") {
		path += "index.html"
	}

	f, err := os.Open(path)
	// File not found - return 404
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("404 - " + http.StatusText(404)))
		return
	}

	// add correct MIME type on response header
	ct := mime.TypeByExtension(filepath.Ext(path))
	w.Header().Add("Content-Type", ct)

	// if html inject js payload
	if strings.HasSuffix(path, ".html") {
		log.Printf("Serving injected %s", path)

		buf := new(bytes.Buffer)
		buf.ReadFrom(f)
		contents := buf.String()
		contents = strings.Replace(contents, "</head>", jsws, 1)

		w.Write([]byte(contents))
	} else {
		log.Printf("Serving %s", path)
		// don't care about content here
		// read it to buffer in order to save memory
		bufferedReader := bufio.NewReader(f)
		// write the file content to the response
		bufferedReader.WriteTo(w)
	}
}

func wsHandler(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil) // error ignored for sake of simplicity

	if err != nil {
		log.Fatal("Fatal error open websocket.")
	}

	for {
		select {
		// watch for events
		case event := <-watcher.Events:
			{
				log.Printf("Recorder event  %s send remote refresh\n", event.Name)
				if err = conn.WriteMessage(websocket.TextMessage, []byte("refresh")); err != nil {
					log.Printf("Error sending refresh")
				}
			}
		// watch for errors
		case err := <-watcher.Errors:
			log.Printf("Error watching file %s", err)
		}
	}
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func watchDir(path string, fi os.FileInfo, err error) error {

	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi.Mode().IsDir() {
		return watcher.Add(path)
	}

	return nil
}

func main() {

	if len(os.Getenv("SITE_FOLDER")) > 0 {
		siteFolder = os.Getenv("SITE_FOLDER")
	}

	if len(os.Getenv("WS_API_URL")) > 0 {
		wsAPIURL = os.Getenv("WS_API_URL")
	}

	if len(os.Getenv("SERVER_PORT")) > 0 {
		port = os.Getenv("SERVER_PORT")
	}

	// JS payload with websocket client
	jsws = fmt.Sprintf(`<script>var socket = new WebSocket("ws://localhost:%s/%s");

	socket.onopen = function () {
		console.log("Status: Connected\n");
	};

	socket.onmessage = async function (e) {
		console.log("Received: " + e.data + "\n");
		await new Promise(r => setTimeout(r, 100));
		location.reload();
	};
	</script>
	</head>`, port, wsAPIURL)

	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories and add watcher
	if err := filepath.Walk(siteFolder, watchDir); err != nil {
		log.Fatalln("Fatal error reading site folder")
	}

	// custom handler to serve file
	http.HandleFunc("/", httpHandler)
	// custom handler for websocket
	http.HandleFunc("/"+wsAPIURL, wsHandler)

	log.Println("Setting site folder :", siteFolder)
	log.Println("Server listening on :", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}
