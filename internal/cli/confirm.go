package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

func shouldPromptForConfirm() bool {
	if isJSONOutput() {
		return false
	}
	return canUseInteractiveTerminal()
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

func confirmationRequiredResult(skipFlag string) commandexec.Result {
	return commandexec.Failure(
		ErrConfirmationRequired,
		"confirmation required",
		nil,
		fmt.Sprintf("Re-run with %s", skipFlag),
	)
}

// confirmDestructive prompts on a TTY. An interactive "no" still reports
// cancelled (exit 0, "Cancelled."). Without a TTY it returns
// CONFIRMATION_REQUIRED instead of treating silence as a successful cancel.
func confirmDestructive(message, skipFlag string) (bool, commandexec.Result) {
	if shouldPromptForConfirm() {
		if promptForConfirm(message) {
			return true, commandexec.Result{}
		}
		return false, commandexec.Success(commandpayload.CancelledResult{Cancelled: true}, nil)
	}
	return false, confirmationRequiredResult(skipFlag)
}

// confirmResponseIsYes reports whether a raw [y/N] prompt response is
// affirmative. Only an explicit "y"/"yes" (case-insensitive) applies; empty,
// "n"/"no", or anything else aborts.
func confirmResponseIsYes(response string) bool {
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
