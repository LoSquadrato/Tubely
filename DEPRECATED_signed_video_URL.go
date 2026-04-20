package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

// DEPRECATED: This file contains functions for signing video URLs, downdated in favor of using CloudFront signed URLs.

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return video, nil
	}
	bucket, key, valid := strings.Cut(*video.VideoURL, ",")
	if !valid {
		return video, fmt.Errorf("invalid video URL format")
	}

	signedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 30*time.Minute)
	if err != nil {
		return video, fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	video.VideoURL = &signedURL
	return video, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s3Client)
	presignRequest, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return presignRequest.URL, nil
}

func (cfg *apiConfig) signVideoURL(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return video, nil
	}
	_, videoKey, isValid := strings.Cut(*video.VideoURL, "amazonaws.com/")
	if !isValid {
		return video, fmt.Errorf("invalid video URL format")
	}
	signedURL := fmt.Sprintf("%s,%s", cfg.s3Bucket, videoKey)
	video.VideoURL = &signedURL

	return video, nil
}
