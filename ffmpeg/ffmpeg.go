package ffmpeg

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"githubo.com/mateo-14/go-yt/ytdlp"
)

func CheckFfmpeg() {
	exists := ffmpegExists()
	if !exists {
		log.Println("FFmpeg not found")
	}

	if runtime.GOOS != "linux" {
		log.Println("Automatic installation of FFmpeg is only supported on Linux")
		return
	}

	res, err := http.Get("https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/latest")
	if err != nil {
		log.Fatal(err)
	}

	if res.StatusCode != 200 {
		log.Fatal("YTDL: Status code was not 200")
	}
	defer res.Body.Close()
	gitRelease := ytdlp.GithubRelease{}
	err = json.NewDecoder(res.Body).Decode(&gitRelease)
	if err != nil {
		log.Fatal(err)
	}

	for _, asset := range gitRelease.Assets {
		if asset.Name == "ffmpeg-master-latest-linux64-gpl.tar.xz" {
			log.Println("Downloading FFmpeg")
			download(asset.DownloadUrl)
			return
		}
	}

}

func GetExecutableName() string {
	os := runtime.GOOS
	if os == "windows" {
		return "ffmpeg.exe"
	}

	return "ffmpeg"
}

func ffmpegExists() bool {
	name := GetExecutableName()
	_, err := os.Stat(name)

	return err == nil
}

func download(url string) {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	out, err := os.Create("ffmpeg.tar.xz")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	_, err = io.Copy(out, res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var errbuf bytes.Buffer
	cmd := exec.Command("tar", "-xvf", "ffmpeg.tar.xz")
	cmd.Stderr = &errbuf
	err = cmd.Run()
	if err != nil {
		log.Fatal(errbuf.String())
	}
}
