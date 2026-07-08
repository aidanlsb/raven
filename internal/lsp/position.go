package lsp

import (
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Position encodings negotiated with the client.
const (
	encodingUTF8  = "utf-8"
	encodingUTF16 = "utf-16"
)

// documentLines splits content into lines without trailing newlines.
func documentLines(content string) []string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

// lineAt returns the 0-indexed line, or "" when out of range.
func lineAt(lines []string, line int) string {
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

// characterToByte converts a character offset in the negotiated encoding to a
// byte offset within line. Offsets past the end of the line clamp to len(line).
func characterToByte(line string, character int, encoding string) int {
	if character <= 0 {
		return 0
	}
	if encoding == encodingUTF8 {
		if character > len(line) {
			return len(line)
		}
		return character
	}

	units := 0
	for i, r := range line {
		if units >= character {
			return i
		}
		units += utf16Len(r)
	}
	return len(line)
}

// byteToCharacter converts a byte offset within line to a character offset in
// the negotiated encoding. Offsets past the end of the line clamp.
func byteToCharacter(line string, byteOffset int, encoding string) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	}
	if encoding == encodingUTF8 {
		return byteOffset
	}

	units := 0
	for i, r := range line {
		if i >= byteOffset {
			break
		}
		units += utf16Len(r)
	}
	return units
}

func utf16Len(r rune) int {
	if r > 0xFFFF && utf8.RuneLen(r) == 4 {
		return 2
	}
	return 1
}

// byteRangeToRange converts a byte-offset span on a 0-indexed line to an LSP range.
func byteRangeToRange(line string, lineNumber, startByte, endByte int, encoding string) Range {
	return Range{
		Start: Position{Line: lineNumber, Character: byteToCharacter(line, startByte, encoding)},
		End:   Position{Line: lineNumber, Character: byteToCharacter(line, endByte, encoding)},
	}
}

// wholeLineRange spans an entire 0-indexed line.
func wholeLineRange(line string, lineNumber int, encoding string) Range {
	return byteRangeToRange(line, lineNumber, 0, len(line), encoding)
}

// pathToURI converts an absolute filesystem path to a file:// URI.
func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		// Windows drive paths (C:/...) need a leading slash.
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

// uriToPath converts a file:// URI to a filesystem path.
// Returns "" when the URI is not a file URI.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	path := u.Path
	// Windows URIs look like file:///C:/path; strip the leading slash.
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
