package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var objectsCmd = newCanonicalLeafCommand("objects", canonicalLeafOptions{
	RenderHuman: renderObjects,
})

func renderObjects(_ *cobra.Command, result commandexec.Result) error {
	payload, ok := result.Data.(commandpayload.ObjectsResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "unexpected objects result shape", "")
	}
	fmt.Println(ui.SectionHeader(fmt.Sprintf("Objects (%d)", payload.Total)))
	if len(payload.Items) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(none)")))
		return nil
	}
	for _, item := range payload.Items {
		label := item.Name
		if label == "" {
			label = item.ID
		}
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s", ui.Bold.Render(label), ui.Hint("("+item.Type+")"), ui.FilePath(item.FilePath))))
	}
	if payload.HasMore {
		next := payload.Offset + payload.Returned
		if payload.NextOffset != nil {
			next = *payload.NextOffset
		}
		fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Showing %d of %d. Pass --limit/--offset to page (next_offset=%d).", payload.Returned, payload.Total, next)))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(objectsCmd)
}
