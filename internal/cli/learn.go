package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/lessons"
	"github.com/aidanlsb/raven/internal/vault"
)

const (
	learnStatusCompleted  = "completed"
	learnStatusIncomplete = "incomplete"
)

type learnPrereqView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Completed   bool   `json:"completed"`
	OpenCommand string `json:"open_command"`
}

type learnLessonView struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Status        string            `json:"status"`
	CompletedDate string            `json:"completed_date,omitempty"`
	Prereqs       []learnPrereqView `json:"prereqs,omitempty"`
}

type learnSectionView struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Lessons []learnLessonView `json:"lessons"`
}

type learnNextView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	OpenCommand string `json:"open_command"`
}

var learnDoneDate string

var learnCmd = &cobra.Command{
	Use:   "learn",
	Short: "Browse and track built-in Raven lessons",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLearnList()
	},
}

var learnListCmd = &cobra.Command{
	Use:   "list",
	Short: "List lesson sections, statuses, and next suggestion",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLearnList()
	},
}

var learnOpenCmd = &cobra.Command{
	Use:   "open <lesson-id>",
	Short: "Open lesson content and advisory prerequisites",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		lessonID := strings.TrimSpace(args[0])

		catalog, err := lessons.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		progress, err := lessons.LoadProgress(vaultPath)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		lesson, ok := catalog.LessonByID(lessonID)
		if !ok {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown lesson: %s", lessonID), "Use 'rvn learn list' to see available lessons")
		}

		prereqs := buildLearnPrereqViews(catalog, progress, lesson)

		if isJSONOutput() {
			lessonData := map[string]interface{}{
				"id":      lesson.ID,
				"title":   lesson.Title,
				"content": lesson.Content,
			}
			if len(lesson.Docs) > 0 {
				lessonData["docs"] = lesson.Docs
			}

			outputSuccess(map[string]interface{}{
				"lesson":  lessonData,
				"prereqs": prereqs,
			}, nil)
			return nil
		}

		fmt.Printf("Lesson: %s (%s)\n\n", lesson.Title, lesson.ID)
		fmt.Print(lesson.Content)
		if lesson.Content != "" && !strings.HasSuffix(lesson.Content, "\n") {
			fmt.Println()
		}

		if len(lesson.Docs) > 0 {
			fmt.Println()
			fmt.Println("Further reading:")
			for _, doc := range lesson.Docs {
				fmt.Printf("- %s\n", doc)
			}
		}

		if len(prereqs) > 0 {
			fmt.Println()
			fmt.Println("Suggested prerequisites:")
			for _, prereq := range prereqs {
				if prereq.Completed {
					fmt.Printf("- %s (%s): completed\n", prereq.ID, prereq.Title)
				} else {
					fmt.Printf("- %s (%s): incomplete, open with `%s`\n", prereq.ID, prereq.Title, prereq.OpenCommand)
				}
			}
		}

		return nil
	},
}

var learnDoneCmd = &cobra.Command{
	Use:   "done <lesson-id>",
	Short: "Mark a lesson complete",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		lessonID := strings.TrimSpace(args[0])

		catalog, err := lessons.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if _, ok := catalog.LessonByID(lessonID); !ok {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown lesson: %s", lessonID), "Use 'rvn learn list' to see available lessons")
		}

		progress, err := lessons.LoadProgress(vaultPath)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		doneAt, err := vault.ParseDateArg(learnDoneDate)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Use YYYY-MM-DD or today/yesterday/tomorrow")
		}
		doneDate := vault.FormatDateISO(doneAt)

		alreadyCompleted := progress.MarkCompleted(lessonID, doneDate)
		if !alreadyCompleted {
			if err := lessons.SaveProgress(vaultPath, progress); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"lesson_id":          lessonID,
				"date":               doneDate,
				"already_completed":  alreadyCompleted,
				"progress_file_path": lessons.ProgressRelPath,
			}, nil)
			return nil
		}

		if alreadyCompleted {
			fmt.Printf("Lesson already completed: %s\n", lessonID)
			return nil
		}

		fmt.Printf("Marked lesson complete: %s (%s)\n", lessonID, doneDate)
		return nil
	},
}

var learnNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Show the next suggested lesson",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		catalog, err := lessons.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		progress, err := lessons.LoadProgress(vaultPath)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		nextLesson, ok := catalog.NextSuggested(progress)
		if !ok {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"all_completed": true,
				}, nil)
				return nil
			}
			fmt.Println("All lessons are complete.")
			return nil
		}

		next := learnNextView{
			ID:          nextLesson.ID,
			Title:       nextLesson.Title,
			OpenCommand: fmt.Sprintf("rvn learn open %s", nextLesson.ID),
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"all_completed": false,
				"next":          next,
			}, nil)
			return nil
		}

		fmt.Printf("Next suggested lesson: %s (%s)\n", next.Title, next.ID)
		fmt.Printf("Open with: %s\n", next.OpenCommand)
		return nil
	},
}

var learnValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate lesson catalog integrity",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		report := lessons.ValidateDefaults()

		if isJSONOutput() {
			if report.Valid {
				outputSuccess(report, nil)
				return nil
			}
			outputError(
				ErrValidationFailed,
				fmt.Sprintf("lesson catalog validation failed (%d error(s))", report.ErrorCount),
				report,
				"Fix lesson files/syllabus and rerun `rvn learn validate`",
			)
			return nil
		}

		if report.Valid {
			fmt.Printf("Lesson catalog is valid (%d sections, %d lessons).\n", report.SectionCount, report.LessonCount)
			if report.WarningCount > 0 {
				fmt.Printf("Warnings: %d\n", report.WarningCount)
				for _, issue := range report.Issues {
					if issue.Severity != lessons.ValidationSeverityWarning {
						continue
					}
					fmt.Printf("- [%s] %s\n", issue.Code, issue.Message)
				}
			}
			return nil
		}

		fmt.Printf("Lesson catalog validation failed (%d error(s), %d warning(s)).\n", report.ErrorCount, report.WarningCount)
		for _, issue := range report.Issues {
			fmt.Printf("- [%s] %s\n", issue.Code, issue.Message)
		}
		return fmt.Errorf("lesson catalog validation failed")
	},
}

func runLearnList() error {
	vaultPath := getVaultPath()

	catalog, err := lessons.LoadCatalog()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	progress, err := lessons.LoadProgress(vaultPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	sections := buildLearnSectionViews(catalog, progress)
	nextLesson, hasNext := catalog.NextSuggested(progress)

	if isJSONOutput() {
		data := map[string]interface{}{
			"sections": sections,
		}
		if hasNext {
			data["next"] = learnNextView{
				ID:          nextLesson.ID,
				Title:       nextLesson.Title,
				OpenCommand: fmt.Sprintf("rvn learn open %s", nextLesson.ID),
			}
		} else {
			data["all_completed"] = true
		}

		outputSuccess(data, nil)
		return nil
	}

	fmt.Println("Raven Lessons")
	fmt.Println()
	for _, section := range sections {
		fmt.Printf("%s\n", section.Title)
		for _, lesson := range section.Lessons {
			marker := "[ ]"
			if lesson.Status == learnStatusCompleted {
				marker = "[x]"
			}
			fmt.Printf("  %s %s (%s)\n", marker, lesson.Title, lesson.ID)
			for _, prereq := range lesson.Prereqs {
				if prereq.Completed {
					fmt.Printf("      suggested prereq %s: completed\n", prereq.ID)
				} else {
					fmt.Printf("      suggested prereq %s: incomplete (%s)\n", prereq.ID, prereq.OpenCommand)
				}
			}
		}
		fmt.Println()
	}

	if hasNext {
		fmt.Printf("Next suggested: %s (%s)\n", nextLesson.Title, fmt.Sprintf("rvn learn open %s", nextLesson.ID))
	} else {
		fmt.Println("All lessons are complete.")
	}

	return nil
}

func buildLearnSectionViews(catalog *lessons.Catalog, progress *lessons.Progress) []learnSectionView {
	out := make([]learnSectionView, 0, len(catalog.Sections))
	for _, section := range catalog.Sections {
		sectionView := learnSectionView{
			ID:      section.ID,
			Title:   section.Title,
			Lessons: make([]learnLessonView, 0, len(section.Lessons)),
		}
		for _, lessonID := range section.Lessons {
			lesson, ok := catalog.LessonByID(lessonID)
			if !ok {
				continue
			}

			view := learnLessonView{
				ID:      lesson.ID,
				Title:   lesson.Title,
				Status:  learnStatusIncomplete,
				Prereqs: buildLearnPrereqViews(catalog, progress, lesson),
			}
			if progress.IsCompleted(lessonID) {
				view.Status = learnStatusCompleted
				if doneDate, ok := progress.CompletedDate(lessonID); ok {
					view.CompletedDate = doneDate
				}
			}
			sectionView.Lessons = append(sectionView.Lessons, view)
		}
		out = append(out, sectionView)
	}
	return out
}

func buildLearnPrereqViews(catalog *lessons.Catalog, progress *lessons.Progress, lesson lessons.Lesson) []learnPrereqView {
	if len(lesson.Prereqs) == 0 {
		return nil
	}

	out := make([]learnPrereqView, 0, len(lesson.Prereqs))
	for _, prereqID := range lesson.Prereqs {
		prereqTitle := prereqID
		if prereqLesson, ok := catalog.LessonByID(prereqID); ok {
			prereqTitle = prereqLesson.Title
		}
		out = append(out, learnPrereqView{
			ID:          prereqID,
			Title:       prereqTitle,
			Completed:   progress.IsCompleted(prereqID),
			OpenCommand: fmt.Sprintf("rvn learn open %s", prereqID),
		})
	}
	return out
}

func init() {
	learnDoneCmd.Flags().StringVar(&learnDoneDate, "date", "", "Completion date (today/yesterday/tomorrow/YYYY-MM-DD)")

	learnCmd.AddCommand(learnListCmd)
	learnCmd.AddCommand(learnOpenCmd)
	learnCmd.AddCommand(learnDoneCmd)
	learnCmd.AddCommand(learnNextCmd)
	learnCmd.AddCommand(learnValidateCmd)

	rootCmd.AddCommand(learnCmd)
}
