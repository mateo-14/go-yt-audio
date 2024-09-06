package main

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/go-sql-driver/mysql"
	"github.com/subosito/gotenv"
	"githubo.com/mateo-14/go-yt/ytdlp"
)

var (
	downloads  = sync.Map{}
	s3Client   *s3.Client
	bucketName string
)

type Download struct {
	wg      *sync.WaitGroup
	success bool
}

type YtVideoData struct {
	Title    string `json:"title"`
	IsLive   bool   `json:"is_live"`
	Duration int    `json:"duration"`
}

func main() {
	log.SetOutput(os.Stdout)
	err := gotenv.Load()
	if err != nil {
		log.Println(err)
	}

	// Get database connection
	db, err := getDatabaseConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	var publicBucket = "https://sn.mateoledesma.com"

	bucketName = os.Getenv("S3_BUCKET_NAME")
	if bucketName == "" {
		log.Fatal("BUCKET_NAME is not set in .env file")
	}

	s3Client, err = getS3Client()
	if err != nil {
		log.Fatal(err)
	}

	// Create downloads table if not exists
	if db.Exec("CREATE TABLE IF NOT EXISTS yt_downloads (id VARCHAR(255) PRIMARY KEY, downloaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, last_accessed TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP);"); err != nil {
		log.Fatal(err)
	}

	ytdlp.CheckYtdl()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{url}", func(w http.ResponseWriter, r *http.Request) {
		url := r.PathValue("url")

		if url == "" || url == "favicon.ico" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid URL"))
			return
		}

		rows, err := db.Query("SELECT id FROM yt_downloads WHERE id = ?", url)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error querying database"))
			log.Println(err)
			return
		}

		if rows.Next() {
			rows.Close()
			_, err := db.Exec("UPDATE yt_downloads SET last_accessed = CURRENT_TIMESTAMP WHERE id = ?", url)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Error updating database"))
				log.Println(err)
				return
			}

			log.Printf("File exists %s", url)
			w.Header().Set("Location", publicBucket+"/"+url+".opus")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}

		var download *Download
		if v, ok := downloads.Load(url); !ok {
			download = &Download{wg: &sync.WaitGroup{}}
			download.wg.Add(1)
			downloads.Store(url, download)

			go func() {
				log.Printf("Downloading %s", url)
				if err := downloadAndUploadAudio(r.Context(), url); err != nil {
					log.Printf("Error downloading %s: %s", url, err)
					download.success = false
					downloads.Delete(url)
					download.wg.Done()
					return
				}
				db.Exec("INSERT INTO yt_downloads (id) VALUES (?)", url)
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

		w.Header().Set("Location", publicBucket+"/"+url+".opus")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", mux)
}

func downloadAndUploadAudio(ctx context.Context, id string) error {
	data, err := getVideoData(id)
	if err != nil {
		return err
	}
	log.Println(data)

	if data.IsLive {
		return fmt.Errorf("video is live")
	}

	if data.Duration > 600 {
		return fmt.Errorf("video is too long")
	}

	piper, pipew := io.Pipe()
	defer piper.Close()
	defer pipew.Close()

	cmd := exec.Command("./"+ytdlp.GetExecutableName(), "-f", "worst*[acodec=opus]", "--embed-metadata", "-x", "-o", "-", fmt.Sprintf("https://www.youtube.com/watch?v=%s", id))
	cmd2 := exec.Command("./ffmpeg.exe", "-i", "pipe:0", "-f", "opus", "-c:a", "libopus", "-b:a", "49k", "-metadata", fmt.Sprintf("title=%s", data.Title), "pipe:1")

	cmd.Stdout = pipew
	cmd2.Stdin = piper

	var buffer bytes.Buffer
	cmd2.Stdout = &buffer

	log.Println("Starting download and conversion")
	cmd.Start()
	cmd2.Start()

	cmd.Wait()
	log.Println("Downloaded")
	pipew.Close()
	cmd2.Wait()
	log.Println("Converted")

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(id + ".opus"),
		Body:        &buffer,
		ContentType: aws.String("audio/ogg"),
	})

	return err
}

func getDatabaseConnection() (*sql.DB, error) {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		return nil, fmt.Errorf("DB_HOST is not set in .env file")
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		return nil, fmt.Errorf("DB_USER is not set in .env file")
	}
	dbDatabase := os.Getenv("DB_DATABASE")
	if dbDatabase == "" {
		return nil, fmt.Errorf("DB_DATABASE is not set in .env file")
	}

	dbPort := cmp.Or(os.Getenv("DB_PORT"), "3306")

	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword != "" {
		dbPassword = ":" + dbPassword
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s%s@tcp(%s:%s)/%s", dbUser, dbPassword, dbHost, dbPort, dbDatabase))
	if err != nil {
		return nil, err
	}

	return db, nil
}

func getS3Client() (*s3.Client, error) {
	accountId := os.Getenv("R2_ACCOUNT_ID")
	if accountId == "" {
		return nil, fmt.Errorf("ACCOUNT_ID is not set in .env file")
	}

	accessKeyId := os.Getenv("S3_ACCESS_KEY_ID")
	if accessKeyId == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY_ID is not set in .env file")
	}

	accessKeySecret := os.Getenv("S3_ACCESS_KEY_SECRET")
	if accessKeySecret == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY_SECRET is not set in .env file")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountId))
	})

	return s3Client, nil
}

func getVideoData(id string) (YtVideoData, error) {
	cmd := exec.Command("./"+ytdlp.GetExecutableName(), "-j", fmt.Sprintf("https://www.youtube.com/watch?v=%s", id))
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return YtVideoData{}, err
	}

	data := YtVideoData{}
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return YtVideoData{}, err
	}

	return data, nil
}
