package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// get video ID from URL
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// get user ID from JWT
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	// parse multipart form data and get file, file header, and content type
	const maxMemory = 1 << 30 // 1 GB
	http.MaxBytesReader(w, r.Body, maxMemory)
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediatype, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse media type", err)
		return
	}
	if mediatype != "video/mp4" && mediatype != "video/mkv" {
		respondWithError(w, http.StatusBadRequest, "Unsupported file type", nil)
		return
	}

	// get video from database and check if it belongs to user
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have permission to upload a video for this video", nil)
		return
	}

	// create temp file and copy uploaded file to it
	fileTmp, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
		return
	}
	defer os.Remove(fileTmp.Name()) // clean up temp file after we're done
	defer fileTmp.Close()

	_, err = io.Copy(fileTmp, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save file", err)
		return
	}

	// generate random key for S3 object and determine prefix based on aspect ratio
	keySpace := make([]byte, 32)
	_, err = rand.Read(keySpace)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate thumbnail ID", err)
		return
	}
	keyFile := base64.RawURLEncoding.EncodeToString(keySpace)
	var prefix string
	if aspectRatio, err := getVideoAspectRatio(fileTmp.Name()); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	} else {
		switch aspectRatio {
		case "16:9":
			prefix = "landscape/"
		case "9:16":
			prefix = "portrait/"
		default:
			prefix = "other/"
		}
	}

	// process video for fast start and open processed file
	processedFile, err := processVideoForFastStart(fileTmp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}
	defer os.Remove(processedFile) // clean up processed file after we're done
	fastStartFile, err := os.Open(processedFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video", err)
		return
	}
	defer fastStartFile.Close()

	// generate key for S3 object
	keyForBucket := prefix + keyFile + filepath.Ext(header.Filename)

	// upload file to S3 (delete existing object if it exists, then put new object)
	fastStartFile.Seek(0, io.SeekStart)
	data, err := io.ReadAll(fastStartFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read file", err)
		return
	}

	cfg.s3Client.DeleteObject(
		context.Background(),
		&s3.DeleteObjectInput{
			Bucket: aws.String(cfg.s3Bucket),
			Key:    aws.String(keyForBucket),
		},
	)

	cfg.s3Client.PutObject(
		context.Background(),
		&s3.PutObjectInput{
			Bucket:      aws.String(cfg.s3Bucket),
			Key:         aws.String(keyForBucket),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(header.Header.Get("Content-Type")),
		},
	)

	videoURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, keyForBucket)
	video.VideoURL = &videoURL
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video with URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
