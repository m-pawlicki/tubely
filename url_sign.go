package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s3Client)
	params := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	presignedObj, err := presignClient.PresignGetObject(context.Background(), params, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return presignedObj.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	url := *video.VideoURL
	vals := strings.Split(url, ",")
	if len(vals) != 2 {
		return database.Video{}, fmt.Errorf("not enough values")
	}
	bucket := vals[0]
	key := vals[1]
	signedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, time.Hour)
	if err != nil {
		return database.Video{}, err
	}
	video.VideoURL = &signedURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		return database.Video{}, fmt.Errorf("couldn't update video")
	}
	return video, nil
}
