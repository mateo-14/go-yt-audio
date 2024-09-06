package ffmpeg

import (
	"archive/tar"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"

	"github.com/ulikunitz/xz"
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

	uncompressedStream, err := xz.NewReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	tarReader := tar.NewReader(uncompressedStream)
	var folder string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			{
				err := os.MkdirAll(header.Name, 0755)
				if err != nil {
					log.Fatal(err)
				}
				folder = header.Name
				break
			}
		case tar.TypeReg:
			{
				out, err := os.Create(path.Join(folder, path.Base(header.Name)))
				if err != nil {
					log.Fatal(err)
				}
				defer out.Close()
			}
		}
	}

	f, err := os.Open(path.Join(folder, "ffmpeg"))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	out, err := os.Create("ffmpeg")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	_, err = io.Copy(out, f)
	if err != nil {
		log.Fatal(err)
	}

	err = out.Sync()
	if err != nil {
		log.Fatal(err)
	}

	err = os.Chmod("ffmpeg", 0755)
	if err != nil {
		log.Fatal(err)
	}

	os.RemoveAll(folder)
}
