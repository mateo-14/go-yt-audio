package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

var (
	downloads = make(map[string]*sync.Cond)
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{url}", func(w http.ResponseWriter, r *http.Request) {
		url := r.PathValue("url")
		if url == "" || url == "favicon.ico" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid URL"))
			return
		}

		if _, ok := downloads[url]; !ok {
			if f, err := os.Open(url + ".opus"); err == nil {
				log.Printf("File already downloaded %s. Streaming...", url)
				streamFile(f, w)
				return
			}

			downloads[url] = sync.NewCond(&sync.Mutex{})
			go func() {
				err := downloadYt(url)
				if err != nil {
					log.Printf("Error downloading %s: %s", url, err)
				}
				downloads[url].L.Lock()
				defer delete(downloads, url)
				defer downloads[url].L.Unlock()
				downloads[url].Broadcast()
			}()
		} else {
			log.Printf("Already downloading %s", url)
		}

		downloads[url].L.Lock()
		defer downloads[url].L.Unlock()
		log.Printf("Waiting for %s", url)
		downloads[url].Wait()

		if f, err := os.Open(url + ".opus"); err == nil {
			log.Printf("File downloaded %s. Streaming...", url)
			streamFile(f, w)
		} else {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
		}

	})

	http.ListenAndServe(":8080", mux)
}

func downloadYt(id string) error {
	cmd := exec.Command("yt-dlp", "-o", "-", "-x", id)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	file, err := os.Create(id + ".opus")
	if err != nil {
		return err
	}

	_, err = io.Copy(file, stdout)
	if err != nil {
		return err
	}

	return nil
}

func streamFile(file *os.File, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "audio/ogg")
	w.Header().Set("Transfer-Encoding", "chunked")

	buff := make([]byte, 1024)
	for {
		n, err := file.Read(buff)
		if err != nil && err != io.EOF {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
			return
		}

		if n == 0 {
			break
		}
		_, err = w.Write(buff[:n])
		if err != nil {
			return
		}

		w.(http.Flusher).Flush()
	}
}
