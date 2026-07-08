package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

// ErrPickCancelled reports that the user cancelled an interactive pick
// without selecting anything. The CLI entry point maps it to exit code 130
// (fzf's abort convention) so pipelines can distinguish "cancelled" from
// "selected nothing".
var ErrPickCancelled = errors.New("pick cancelled")

var (
	pickRun      = picker.Run
	pickRunMulti = picker.RunMulti
	pickOpenTTY  = func() (*os.File, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
)

var pickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Interactively select IDs from pipe-friendly input",
	Long: `Interactively select IDs from pipe-friendly input.

Reads pipe-friendly rows from stdin, such as output from 'rvn query --pipe',
opens Raven's picker on the controlling terminal, and writes selected IDs to
stdout for downstream commands.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if isJSONOutput() {
			return handleErrorMsg(ErrInvalidInput, "--json cannot be used with pick", "Use pipe output or remove --json")
		}
		if term.IsTerminal(os.Stdin.Fd()) {
			return handleErrorMsg(ErrInvalidInput, "pick reads pipe-friendly input from stdin", "Example: rvn query 'trait:todo' --pipe | rvn pick --multi")
		}

		items, err := readPickItems(os.Stdin)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if len(items) == 0 {
			return handleErrorMsg(ErrInvalidInput, "no selectable items on stdin", "Pipe output from a list command, e.g. rvn query 'type:project' --pipe")
		}

		tty, err := pickOpenTTY()
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, "interactive pick requires a controlling terminal", "Run from an interactive terminal")
		}
		defer tty.Close()

		multi, _ := cmd.Flags().GetBool("multi")
		opts := picker.Options{
			Title:   "Pick items",
			Prompt:  "filter",
			Headers: []string{"#", "content", "id", "location"},
			Columns: ui.SearchLayout(),
			Preview: vaultFilePreview(getVaultPath()),
			Input:   tty,
			Output:  tty,
		}
		if multi {
			selections, ok, err := pickRunMulti(items, opts)
			if err != nil {
				return handleError(ErrInternal, err, "")
			}
			if !ok {
				return pickCancelled(cmd)
			}
			for _, selection := range selections {
				fmt.Println(selection.Item.ID)
			}
			return nil
		}

		selection, ok, err := pickRun(items, opts)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if !ok {
			return pickCancelled(cmd)
		}
		fmt.Println(selection.Item.ID)
		return nil
	},
}

// pickCancelled silences Cobra's error/usage output and returns the
// cancellation sentinel; main translates it into exit code 130.
func pickCancelled(cmd *cobra.Command) error {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return ErrPickCancelled
}

func readPickItems(r io.Reader) ([]picker.Item, error) {
	scanner := bufio.NewScanner(r)
	items := make([]picker.Item, 0)
	for scanner.Scan() {
		item, ok := pickItemFromLine(scanner.Text())
		if ok {
			items = append(items, item)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read pick input: %w", err)
	}
	return items, nil
}

func pickItemFromLine(line string) (picker.Item, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return picker.Item{}, false
	}

	id := line
	content := ""
	location := ""
	if strings.Contains(line, "\t") {
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			id = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			content = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			location = strings.TrimSpace(parts[3])
		}
	}
	if id == "" {
		return picker.Item{}, false
	}

	label := content
	if label == "" {
		label = id
	}
	filePath, lineNumber := parsePreviewLocation(location)
	searchText := browseSearchText(id, content, location)
	return picker.Item{
		ID:         id,
		Label:      label,
		Detail:     id,
		Location:   location,
		Columns:    []string{label, id, location},
		SearchText: searchText,
		FilePath:   filePath,
		Line:       lineNumber,
	}, true
}

func init() {
	pickCmd.Flags().Bool("multi", false, "Allow selecting multiple items")
	markLocalLeaf(pickCmd)
	rootCmd.AddCommand(pickCmd)
}
