package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

var (
	downloads = sync.Map{}
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

		var wg *sync.WaitGroup
		if v, ok := downloads.Load(url); !ok {
			if f, err := os.Open(url + ".opus"); err == nil {
				log.Printf("File already downloaded %s. Streaming...", url)
				streamFile(f, w)
				return
			}

			wg = &sync.WaitGroup{}
			wg.Add(1)
			downloads.Store(url, wg)

			go func() {
				err := downloadYt(url)
				if err != nil {
					log.Printf("Error downloading %s: %s", url, err)
				}
				downloads.Delete(url)
				wg.Done()
			}()
		} else {
			wg = v.(*sync.WaitGroup)
			log.Printf("Already downloading %s", url)
		}

		log.Printf("Downloading %s", url)
		wg.Wait()

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
	cmd := exec.Command("yt-dlp", "-o", "-", "-x", fmt.Sprintf("https://www.youtube.com/watch?v=%s", id))
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
	fsstat, _ := file.Stat()
	filesize := fsstat.Size()
	w.Header().Set("Content-Length", strconv.Itoa(int(filesize)))
	io.Copy(w, file)
}
