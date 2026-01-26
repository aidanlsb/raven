package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/aidanlsb/raven/internal/ui"
)

func shouldPromptForConfirm() bool {
	if isJSONOutput() {
		return false
	}
	return isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd())
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
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
