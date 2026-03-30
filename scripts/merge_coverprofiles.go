package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/merge_coverprofiles.go <output> <input> [<input>...]")
		os.Exit(1)
	}

	outputPath := os.Args[1]
	inputPaths := os.Args[2:]

	mode := ""
	counts := make(map[string]int64)

	for _, path := range inputPaths {
		if err := mergeProfile(path, &mode, counts); err != nil {
			fmt.Fprintf(os.Stderr, "merge %s: %v\n", path, err)
			os.Exit(1)
		}
	}

	if mode == "" {
		fmt.Fprintln(os.Stderr, "no coverage mode found in inputs")
		os.Exit(1)
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	if _, err := fmt.Fprintf(writer, "mode: %s\n", mode); err != nil {
		fmt.Fprintf(os.Stderr, "write mode: %v\n", err)
		os.Exit(1)
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(writer, "%s %d\n", key, counts[key]); err != nil {
			fmt.Fprintf(os.Stderr, "write entry: %v\n", err)
			os.Exit(1)
		}
	}
	if err := writer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush output: %v\n", err)
		os.Exit(1)
	}
}

func mergeProfile(path string, mode *string, counts map[string]int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if lineNo == 1 {
			currentMode, found := strings.CutPrefix(line, "mode: ")
			if !found {
				return fmt.Errorf("line 1: missing mode header")
			}
			if *mode == "" {
				*mode = currentMode
			} else if *mode != currentMode {
				return fmt.Errorf("mode mismatch: got %q want %q", currentMode, *mode)
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			return fmt.Errorf("line %d: invalid profile entry", lineNo)
		}
		count, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Errorf("line %d: parse count: %w", lineNo, err)
		}
		key := parts[0] + " " + parts[1]
		counts[key] += count
	}
	return scanner.Err()
}
