package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// Spinner displays an animated spinner with a message.
type Spinner struct {
	message string
	frames  []string
	done    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	current int
}

// Default spinner frames (dots style)
var defaultFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		frames:  defaultFrames,
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	// Only animate if we're in a TTY
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("%s...\n", s.message)
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.done:
				// Clear the spinner line
				fmt.Print("\r\033[K")
				return
			case <-ticker.C:
				s.mu.Lock()
				frame := s.frames[s.current%len(s.frames)]
				s.current++
				s.mu.Unlock()
				fmt.Printf("\r%s %s %s", Bold.Render(frame), s.message, Muted.Render(""))
			}
		}
	}()
}

// Stop stops the spinner and optionally shows a final message.
func (s *Spinner) Stop() {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return
	}
	close(s.done)
	s.wg.Wait()
}

// StopWithMessage stops the spinner and prints a final message.
func (s *Spinner) StopWithMessage(message string) {
	s.Stop()
	fmt.Println(message)
}

// StopWithCheck stops the spinner and prints a success message.
func (s *Spinner) StopWithCheck(message string) {
	s.Stop()
	fmt.Println(Check(message))
}

// Progress displays a simple progress indicator for counted operations.
type Progress struct {
	total   int
	current int
	message string
	mu      sync.Mutex
}

// NewProgress creates a new progress indicator.
func NewProgress(message string, total int) *Progress {
	return &Progress{
		message: message,
		total:   total,
	}
}

// Update updates the progress indicator.
func (p *Progress) Update(current int) {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return
	}
	p.mu.Lock()
	p.current = current
	p.mu.Unlock()
	fmt.Printf("\r%s %s", p.message, Muted.Render(fmt.Sprintf("(%d/%d)", current, p.total)))
}

// Increment increments the progress by one.
func (p *Progress) Increment() {
	p.mu.Lock()
	p.current++
	current := p.current
	p.mu.Unlock()
	if isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("\r%s %s", p.message, Muted.Render(fmt.Sprintf("(%d/%d)", current, p.total)))
	}
}

// Done finishes the progress indicator.
func (p *Progress) Done() {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Print("\r\033[K") // Clear line
	}
}

// DoneWithMessage finishes the progress and prints a message.
func (p *Progress) DoneWithMessage(message string) {
	p.Done()
	fmt.Println(message)
}
