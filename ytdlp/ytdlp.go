package ytdlp

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

type GithubReleaseAsset struct {
	Name        string `json:"name"`
	DownloadUrl string `json:"browser_download_url"`
}

type GithubRelease struct {
	Name    string               `json:"name"`
	TagName string               `json:"tag_name"`
	Assets  []GithubReleaseAsset `json:"assets"`
}

func CheckYtdl() {
	currVersion := getCurrentVersion()

	res, err := http.Get("https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest")
	if err != nil {
		log.Fatal(err)
	}

	if res.StatusCode != 200 {
		log.Fatal("YTDL: Status code was not 200")
	}

	defer res.Body.Close()
	gitRelease := GithubRelease{}
	err = json.NewDecoder(res.Body).Decode(&gitRelease)
	if err != nil {
		log.Fatal(err)
	}

	if currVersion == gitRelease.TagName {
		log.Println("YTDL: Already up to date")
		return
	} else if currVersion != "" {
		log.Println("YTDL: New version available")
		log.Println("YTDL: Current version: " + currVersion)
		log.Println("YTDL: Latest version: " + gitRelease.TagName)
		os.Remove(GetExecutableName())
	}

	log.Println("YTDL: Downloading latest version")
	for _, asset := range gitRelease.Assets {
		if asset.Name == GetExecutableName() {
			downloadFile(asset.DownloadUrl)
			return
		}
	}
	log.Fatal("YTDL: Executable not found in assets")
}

func GetExecutableName() string {
	os := runtime.GOOS
	if os == "windows" {
		return "yt-dlp.exe"
	} else if os == "linux" {
		return "yt-dlp_linux"
	} else if os == "darwin" {
		return "yt-dlp_macos"
	}

	return "yt-dlp"
}

func getCurrentVersion() string {
	name := GetExecutableName()
	_, err := os.Stat(name)
	if err != nil {
		log.Println("Yt-dlp not found")
		return ""
	}

	cmd := exec.Command("./"+name, "--version")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	return string(out)[:len(string(out))-2]
}

func downloadFile(url string) {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	if res.StatusCode != 200 {
		log.Fatal("YTDL: Status code was not 200")
	}

	defer res.Body.Close()
	name := GetExecutableName()
	file, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()
	_, err = file.ReadFrom(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("YTDL: Latest version downloaded")
}
