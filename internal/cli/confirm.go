package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/ui"
)

func shouldPromptForConfirm() bool {
	if isJSONOutput() {
		return false
	}
	return term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stdin.Fd())
}

func promptForConfirm(message string) bool {
	if !shouldPromptForConfirm() {
		return false
	}
	if message == "" {
		message = "Apply changes?"
	}
	fmt.Printf("%s %s ", message, ui.Hint("[y/N]"))
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	return confirmResponseIsYes(response)
}

// confirmResponseIsYes reports whether a raw [y/N] prompt response is
// affirmative. Only an explicit "y"/"yes" (case-insensitive) applies; empty,
// "n"/"no", or anything else aborts.
func confirmResponseIsYes(response string) bool {
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
