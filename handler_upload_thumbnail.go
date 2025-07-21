package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	// TODO: implement the upload here
	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}

	defer file.Close()

	fileType := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(fileType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file type", err)
		return
	}

	if !strings.Contains(mediaType, "image") {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}
	bytes := make([]byte, 32)
	_, err = rand.Read(bytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read bytes", err)
		return
	}
	encoder := base64.RawURLEncoding
	base := encoder.EncodeToString(bytes)
	ext := strings.Split(mediaType, "image/")[1]
	img := fmt.Sprintf("%s.%s", base, ext)

	meta, err := cfg.db.GetVideo(videoID)
	if userID != meta.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unathorized", err)
		return
	}

	filePath := filepath.Join(cfg.assetsRoot, img)
	dst, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't create file", err)
		return
	}
	io.Copy(dst, file)
	dataURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, img)
	meta.ThumbnailURL = &dataURL

	err = cfg.db.UpdateVideo(meta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, meta)
}
