package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aidanlsb/raven/internal/picker"
)

const previewContextLines = 8

func vaultFilePreview(vaultPath string) picker.PreviewFunc {
	return func(item picker.Item) (picker.Preview, error) {
		relPath := strings.TrimSpace(item.FilePath)
		line := item.Line
		if relPath == "" {
			relPath, line = parsePreviewLocation(item.Location)
		}
		if relPath == "" {
			return picker.Preview{}, fmt.Errorf("selected item has no file path")
		}

		cleanPath := filepath.Clean(relPath)
		if filepath.IsAbs(cleanPath) || cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." {
			return picker.Preview{}, fmt.Errorf("preview path must be vault-relative")
		}

		content, err := os.ReadFile(filepath.Join(vaultPath, cleanPath))
		if err != nil {
			return picker.Preview{}, err
		}
		if !utf8.Valid(content) {
			return picker.Preview{}, fmt.Errorf("preview is only available for text files")
		}

		title := cleanPath
		if line > 0 {
			title = fmt.Sprintf("%s:%d", cleanPath, line)
		}
		return picker.Preview{
			Title:   title,
			Content: previewExcerpt(string(content), line),
		}, nil
	}
}

func parsePreviewLocation(location string) (string, int) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", 0
	}
	idx := strings.LastIndex(location, ":")
	if idx < 0 || idx == len(location)-1 {
		return location, 0
	}
	line, err := strconv.Atoi(strings.TrimSpace(location[idx+1:]))
	if err != nil || line <= 0 {
		return location, 0
	}
	return strings.TrimSpace(location[:idx]), line
}

func previewExcerpt(content string, line int) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}

	start := 1
	end := len(lines)
	if line > 0 {
		if line > len(lines) {
			line = len(lines)
		}
		start = line - previewContextLines
		if start < 1 {
			start = 1
		}
		end = line + previewContextLines
		if end > len(lines) {
			end = len(lines)
		}
	} else if end > previewContextLines*2+1 {
		end = previewContextLines*2 + 1
	}

	out := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		marker := " "
		if line > 0 && i == line {
			marker = ">"
		}
		out = append(out, fmt.Sprintf("%s %4d │ %s", marker, i, lines[i-1]))
	}
	return strings.Join(out, "\n")
}
