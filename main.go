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

	"githubo.com/mateo-14/go-yt/ytdlp"
)

var (
	downloads = sync.Map{}
)

type Download struct {
	wg      *sync.WaitGroup
	success bool
}

func main() {
	log.SetOutput(os.Stdout)
	ytdlp.CheckYtdl()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{url}", func(w http.ResponseWriter, r *http.Request) {
		url := r.PathValue("url")

		if url == "" || url == "favicon.ico" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid URL"))
			return
		}

		var download *Download
		if v, ok := downloads.Load(url); !ok {
			if f, err := os.Open(url + ".opus"); err == nil {
				log.Printf("File already downloaded %s. Streaming...", url)
				streamFile(f, w)
				return
			}

			download = &Download{wg: &sync.WaitGroup{}}
			download.wg.Add(1)
			downloads.Store(url, download)

			go func() {
				log.Printf("Downloading %s", url)
				err := downloadYt(url)
				if err != nil {
					log.Printf("Error downloading %s: %s", url, err)
					downloads.Delete(url)
					download.wg.Done()
					return
				}

				download.success = true
				downloads.Delete(url)
				download.wg.Done()
			}()
		} else {
			download = v.(*Download)
			log.Printf("Already downloading %s", url)
		}

		download.wg.Wait()
		if !download.success {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error downloading audio"))
			return
		}

		if f, err := os.Open(url + ".opus"); err == nil {
			log.Printf("File downloaded %s. Streaming...", url)
			defer f.Close()
			streamFile(f, w)
		} else {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
		}
	})

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", mux)
}

func downloadYt(id string) error {
	cmd := exec.Command("./"+ytdlp.GetExecutableName(), "-f", "worst*[acodec=opus]", "--embed-metadata", "-x", "-o", id+".%(ext)s",
		fmt.Sprintf("https://www.youtube.com/watch?v=%s", id))

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
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
