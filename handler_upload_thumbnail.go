package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"io"
	"mime"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	file, header, err := r.FormFile("thumbnail")
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
	if mediatype != "image/jpeg" && mediatype != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unsupported file type", nil)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have permission to upload a thumbnail for this video", nil)
		return
	}

	thumbnailID := make([]byte, 32)
	_, err = rand.Read(thumbnailID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate thumbnail ID", err)
		return
	}
	thumbnailIDString := base64.RawURLEncoding.EncodeToString(thumbnailID)

	filePath := filepath.Join(cfg.assetsRoot, thumbnailIDString+filepath.Ext(header.Filename))
	fileIO, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
		return
	}
	defer fileIO.Close()

	_, err = io.Copy(fileIO, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save file", err)
		return
	}

	dataURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, thumbnailIDString+filepath.Ext(header.Filename))
	video.ThumbnailURL = &dataURL
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video with thumbnail URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
