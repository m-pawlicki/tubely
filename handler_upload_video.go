package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"

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

	const maxMemory = 10 << 30
	http.MaxBytesReader(w, r.Body, maxMemory)

	meta, err := cfg.db.GetVideo(videoID)
	if userID != meta.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unathorized", err)
		return
	}

	file, header, err := r.FormFile("video")
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

	if !strings.Contains(mediaType, "video/mp4") {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	io.Copy(tempFile, file)
	tempFile.Seek(0, io.SeekStart)

	bytes := make([]byte, 32)
	_, err = rand.Read(bytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read bytes", err)
		return
	}
	encoder := base64.RawURLEncoding
	base := encoder.EncodeToString(bytes)
	ext := strings.Split(mediaType, "video/")[1]
	vid := fmt.Sprintf("%s.%s", base, ext)

	ratio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	var aspect string
	switch ratio {
	case "16:9":
		aspect = "landscape"
	case "9:16":
		aspect = "portrait"
	default:
		aspect = "other"
	}

	key := fmt.Sprintf("%s/%s", aspect, vid)

	objParams := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &key,
		Body:        tempFile,
		ContentType: &fileType,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &objParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to put object in s3 bucket", err)
		return
	}

	vidURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/%s", cfg.s3Bucket, cfg.s3Region, aspect, vid)
	meta.VideoURL = &vidURL

	err = cfg.db.UpdateVideo(meta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, meta)
}

func getVideoAspectRatio(filePath string) (string, error) {

	type Video struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	commandOut := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buf bytes.Buffer
	commandOut.Stdout = &buf
	err := commandOut.Run()
	if err != nil {
		return "", err
	}

	aspectRatio := Video{}
	err = json.Unmarshal(buf.Bytes(), &aspectRatio)
	if err != nil {
		return "", err
	}

	width := aspectRatio.Streams[0].Width
	height := aspectRatio.Streams[0].Height
	ratio := width / height
	switch ratio {
	case 1:
		return "16:9", nil
	case 0:
		return "9:16", nil
	default:
		return "other", nil
	}
}
