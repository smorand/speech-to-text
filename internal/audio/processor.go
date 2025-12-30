package audio

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GetDuration returns the duration of an audio file in seconds
func GetDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get audio duration: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// GetMimeType returns MIME type for audio extension
func GetMimeType(ext string) string {
	mimeTypes := map[string]string{
		".aac":  "audio/aac",
		".flac": "audio/flac",
		".m4a":  "audio/mp4",
		".mp3":  "audio/mp3",
		".ogg":  "audio/ogg",
		".wav":  "audio/wav",
	}

	if mime, ok := mimeTypes[strings.ToLower(ext)]; ok {
		return mime
	}
	return "audio/mp3"
}

// SplitIntoChunks splits audio file into chunks with overlaps before and after
func SplitIntoChunks(audioPath string, totalDuration float64, chunkDurationMinutes, overlapSeconds int, logFn func(string)) ([]string, error) {
	// Convert chunk duration from minutes to seconds, overlap is already in seconds
	chunkDuration := float64(chunkDurationMinutes * 60)
	overlap := float64(overlapSeconds)

	// Calculate number of chunks needed
	numChunks := int(math.Ceil(totalDuration / chunkDuration))

	// Create temporary directory with random name
	tempDir, err := createTempDir()
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	var chunkFiles []string
	ext := filepath.Ext(audioPath)

	for i := 0; i < numChunks; i++ {
		var startTime, duration float64

		if i == 0 {
			// First chunk: 0 to (chunkDuration + overlap)
			startTime = 0
			duration = math.Min(chunkDuration+overlap, totalDuration)
		} else if i == numChunks-1 {
			// Last chunk: (prevEnd - overlap) to end
			startTime = float64(i)*chunkDuration - overlap
			duration = totalDuration - startTime
		} else {
			// Middle chunks: (prevEnd - overlap) to (chunkDuration + 2*overlap)
			startTime = float64(i)*chunkDuration - overlap
			duration = chunkDuration + 2*overlap
		}

		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d%s", i, ext))

		if logFn != nil {
			logFn(fmt.Sprintf("Creating chunk %d/%d - start: %.1fs (%.1fmin), duration: %.1fs (%.1fmin)",
				i+1, numChunks, startTime, startTime/60, duration, duration/60))
		}

		if err := createChunk(audioPath, chunkPath, startTime, duration); err != nil {
			return nil, fmt.Errorf("failed to create chunk %d: %w", i, err)
		}

		chunkFiles = append(chunkFiles, chunkPath)
	}

	return chunkFiles, nil
}

// createTempDir creates a temporary directory with a random name
func createTempDir() (string, error) {
	// Generate random directory name
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	randomName := "speech-to-text-" + hex.EncodeToString(randomBytes)

	// Create directory in system temp
	tempDir := filepath.Join(os.TempDir(), randomName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", err
	}

	return tempDir, nil
}

// createChunk creates a single audio chunk using ffmpeg
func createChunk(sourcePath, outputPath string, startTime, duration float64) error {
	cmd := exec.Command("ffmpeg",
		"-i", sourcePath,
		"-ss", fmt.Sprintf("%.1f", startTime),
		"-t", fmt.Sprintf("%.1f", duration),
		"-acodec", "copy",
		"-y", outputPath,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg execution failed: %w", err)
	}
	return nil
}
