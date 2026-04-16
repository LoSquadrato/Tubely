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
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have permission to upload a video for this video", nil)
		return
	}

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

	keyForBucket := prefix + keyFile + filepath.Ext(header.Filename)

	fileTmp.Seek(0, io.SeekStart)
	data, err := io.ReadAll(fileTmp)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read file", err)
		return
	}

	cfg.s3Client.PutObject(
		context.Background(),
		&s3.PutObjectInput{
			Bucket:      aws.String(cfg.s3Bucket),
			Key:         aws.String(keyForBucket),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(header.Header.Get("Content-Type")),
		},
	)

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyForBucket)
	video.VideoURL = &videoURL
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video with URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
