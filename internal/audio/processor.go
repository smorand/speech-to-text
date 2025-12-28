package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ChunkDurationSeconds = 30 * 60 // 30 minutes
	OverlapSeconds       = 30      // 30 seconds overlap
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

// SplitIntoChunks splits audio file into chunks with overlaps
func SplitIntoChunks(audioPath string, numChunks int, logFn func(string)) ([]string, error) {
	var chunkFiles []string
	ext := filepath.Ext(audioPath)

	for i := 0; i < numChunks; i++ {
		startTime := calculateStartTime(i)
		chunkPath := filepath.Join(os.TempDir(), fmt.Sprintf("chunk_%d%s", i, ext))
		duration := float64(ChunkDurationSeconds + OverlapSeconds)

		if logFn != nil {
			logFn(fmt.Sprintf("Creating chunk %d/%d - start: %.1fs, duration: %.1fs",
				i+1, numChunks, startTime, duration))
		}

		if err := createChunk(audioPath, chunkPath, startTime, duration); err != nil {
			return nil, fmt.Errorf("failed to create chunk %d: %w", i, err)
		}

		chunkFiles = append(chunkFiles, chunkPath)
	}

	return chunkFiles, nil
}

// calculateStartTime computes the start time for a given chunk index
func calculateStartTime(chunkIndex int) float64 {
	if chunkIndex == 0 {
		return 0
	}
	return float64(chunkIndex*ChunkDurationSeconds) - OverlapSeconds
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
