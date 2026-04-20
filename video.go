package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	cmd.Stdin = strings.NewReader("some input")
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffprobe failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return "", fmt.Errorf("failed to parse ffprobe output: %w", err)
	}
	var aspectRatio string
	if streams, ok := result["streams"].([]interface{}); ok && len(streams) > 0 {
		for _, stream := range streams {
			if streamMap, ok := stream.(map[string]interface{}); ok {
				if streamMap["codec_type"] == "video" {
					if w, ok := streamMap["display_aspect_ratio"].(string); ok {
						aspectRatio = w
						break
					}
				}
			}
		}
	} else {
		return "", fmt.Errorf("no streams found in ffprobe output")
	}

	return aspectRatio, nil

}

func processVideoForFastStart(filePath string) (string, error) {
	outputPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".processed" + filepath.Ext(filePath)
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)
	cmd.Stdin = strings.NewReader("some input")
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	return outputPath, nil
}
