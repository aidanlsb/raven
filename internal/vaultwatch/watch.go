// Package vaultwatch watches a Raven vault and projects filesystem changes
// into the derived index so companion UIs can poll a cheap revision.
package vaultwatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const (
	// DefaultDebounce is the quiet period after a burst of filesystem events
	// before the index is updated.
	DefaultDebounce = 200 * time.Millisecond
)

// Batch is a debounced set of vault-relative paths, or an overflow that
// requires a full incremental pass.
type Batch struct {
	Paths    []string
	Overflow bool
}

// Options configures a vault watcher.
type Options struct {
	VaultPath  string
	Debounce   time.Duration
	BurstLimit int
	Exclude    *ravenignore.Matcher
	Apply      func(context.Context, Batch) error
	Logf       func(string, ...any)
	Watch      *fsnotify.Watcher
}

// Watcher recursively watches a vault and projects observed changes.
type Watcher struct {
	vaultPath  string
	debounce   time.Duration
	burstLimit int
	exclude    *ravenignore.Matcher
	apply      func(context.Context, Batch) error
	logf       func(string, ...any)
	watch      *fsnotify.Watcher
	ownsWatch  bool

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	flush   chan Batch

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Start watches vaultPath until ctx is cancelled or Close is called.
func Start(ctx context.Context, opts Options) (*Watcher, error) {
	vaultPath := strings.TrimSpace(opts.VaultPath)
	if vaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return nil, err
	}

	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	burstLimit := opts.BurstLimit
	if burstLimit <= 0 {
		burstLimit = reindexsvc.DefaultObserveBurst
	}
	apply := opts.Apply
	if apply == nil {
		apply = defaultApply(abs, burstLimit)
	}
	logf := opts.Logf
	if logf == nil {
		logf = log.Printf
	}

	watch := opts.Watch
	ownsWatch := false
	if watch == nil {
		watch, err = fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("create vault watcher: %w", err)
		}
		ownsWatch = true
	}

	runCtx, cancel := context.WithCancel(ctx)
	w := &Watcher{
		vaultPath:  abs,
		debounce:   debounce,
		burstLimit: burstLimit,
		exclude:    opts.Exclude,
		apply:      apply,
		logf:       logf,
		watch:      watch,
		ownsWatch:  ownsWatch,
		pending:    make(map[string]struct{}),
		flush:      make(chan Batch, 1),
		ctx:        runCtx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	if err := w.addTree(abs); err != nil {
		cancel()
		if ownsWatch {
			_ = watch.Close()
		}
		return nil, err
	}
	go w.loop()
	return w, nil
}

// Close stops the watcher and waits for the loop to exit.
func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	w.cancel()
	<-w.done
	if w.ownsWatch && w.watch != nil {
		return w.watch.Close()
	}
	return nil
}

func defaultApply(vaultPath string, burstLimit int) func(context.Context, Batch) error {
	return func(ctx context.Context, batch Batch) error {
		rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{OpenDB: true, RequireSchema: true})
		if err != nil {
			return err
		}
		defer rt.Close()
		return reindexsvc.ObserveFiles(rt, reindexsvc.ObserveRequest{
			Paths:      batch.Paths,
			Overflow:   batch.Overflow,
			BurstLimit: burstLimit,
			Context:    ctx,
		})
	}
}

func (w *Watcher) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.ctx.Done():
			w.stopTimer()
			return
		case event, ok := <-w.watch.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watch.Errors:
			if !ok {
				return
			}
			if err == nil {
				continue
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.queueOverflow()
				continue
			}
			w.logf("raven api: vault watcher: %v", err)
		case batch := <-w.flush:
			w.runApply(batch)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Op&fsnotify.Chmod == event.Op {
		return
	}
	relPath, skip := w.relPath(event.Name)
	if skip {
		return
	}
	info, err := os.Stat(event.Name)
	isDir := err == nil && info.IsDir()
	if event.Has(fsnotify.Create) && isDir {
		if err := w.watch.Add(event.Name); err != nil {
			w.logf("raven api: watch directory %s: %v", relPath, err)
		}
	}
	if !isInterestingPath(relPath, isDir) {
		return
	}
	w.queuePath(relPath)
}

func (w *Watcher) relPath(name string) (string, bool) {
	rel, err := filepath.Rel(w.vaultPath, name)
	if err != nil {
		return "", true
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", true
	}
	if ignoredWatchPath(rel, w.exclude) {
		return "", true
	}
	return rel, false
}

func (w *Watcher) queuePath(relPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[relPath] = struct{}{}
	w.armTimerLocked()
}

func (w *Watcher) queueOverflow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = map[string]struct{}{}
	w.stopTimerLocked()
	w.emitLocked(Batch{Overflow: true})
}

func (w *Watcher) armTimerLocked() {
	if w.timer != nil {
		w.timer.Reset(w.debounce)
		return
	}
	w.timer = time.AfterFunc(w.debounce, w.fire)
}

func (w *Watcher) fire() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		w.timer = nil
		return
	}
	paths := make([]string, 0, len(w.pending))
	for relPath := range w.pending {
		paths = append(paths, relPath)
	}
	w.pending = make(map[string]struct{})
	w.timer = nil
	w.emitLocked(Batch{Paths: paths})
}

func (w *Watcher) emitLocked(batch Batch) {
	select {
	case w.flush <- batch:
	case <-w.ctx.Done():
	default:
		select {
		case prior := <-w.flush:
			w.flush <- mergeBatches(prior, batch)
		case w.flush <- batch:
		case <-w.ctx.Done():
		}
	}
}

func (w *Watcher) runApply(batch Batch) {
	if err := w.apply(w.ctx, batch); err != nil && !errors.Is(err, context.Canceled) {
		w.logf("raven api: vault watcher apply: %v", err)
	}
}

func (w *Watcher) stopTimer() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopTimerLocked()
}

func (w *Watcher) stopTimerLocked() {
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

func (w *Watcher) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, skip := w.relPath(path)
		if skip && path != root {
			return filepath.SkipDir
		}
		if rel != "" && w.exclude != nil && w.exclude.Match(rel, true) {
			return filepath.SkipDir
		}
		return w.watch.Add(path)
	})
}

func mergeBatches(a, b Batch) Batch {
	if a.Overflow || b.Overflow {
		return Batch{Overflow: true}
	}
	seen := make(map[string]struct{}, len(a.Paths)+len(b.Paths))
	out := make([]string, 0, len(a.Paths)+len(b.Paths))
	for _, group := range [][]string{a.Paths, b.Paths} {
		for _, relPath := range group {
			if _, ok := seen[relPath]; ok {
				continue
			}
			seen[relPath] = struct{}{}
			out = append(out, relPath)
		}
	}
	return Batch{Paths: out}
}

func isInterestingPath(relPath string, isDir bool) bool {
	if isDir {
		return true
	}
	base := pathBase(relPath)
	return paths.HasMDExtension(relPath) || base == paths.SchemaFilename || base == "raven.yaml"
}

func pathBase(relPath string) string {
	if i := strings.LastIndex(relPath, "/"); i >= 0 {
		return relPath[i+1:]
	}
	return relPath
}

func ignoredWatchPath(relPath string, exclude *ravenignore.Matcher) bool {
	if relPath == ".raven" || strings.HasPrefix(relPath, ".raven/") {
		return true
	}
	if relPath == ".git" || strings.HasPrefix(relPath, ".git/") {
		return true
	}
	if relPath == ".trash" || strings.HasPrefix(relPath, ".trash/") {
		return true
	}
	if exclude != nil && exclude.Match(relPath, strings.HasSuffix(relPath, "/")) {
		return true
	}
	return false
}
