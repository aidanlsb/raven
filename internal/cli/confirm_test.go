package cli

import "testing"

func TestConfirmResponseIsYes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{name: "y applies", response: "y\n", want: true},
		{name: "yes applies", response: "yes\n", want: true},
		{name: "uppercase Y applies", response: "Y\n", want: true},
		{name: "mixed-case Yes applies", response: "  Yes  \n", want: true},
		{name: "empty aborts", response: "\n", want: false},
		{name: "n aborts", response: "n\n", want: false},
		{name: "no aborts", response: "no\n", want: false},
		{name: "other aborts", response: "maybe\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := confirmResponseIsYes(tt.response); got != tt.want {
				t.Fatalf("confirmResponseIsYes(%q) = %v, want %v", tt.response, got, tt.want)
			}
		})
	}
}
