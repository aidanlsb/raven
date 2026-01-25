package lastquery

import (
	"reflect"
	"testing"
)

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{
			name:  "single number",
			input: "1",
			want:  []int{1},
		},
		{
			name:  "comma separated",
			input: "1,3,5",
			want:  []int{1, 3, 5},
		},
		{
			name:  "space separated",
			input: "1 3 5",
			want:  []int{1, 3, 5},
		},
		{
			name:  "range",
			input: "1-5",
			want:  []int{1, 2, 3, 4, 5},
		},
		{
			name:  "mixed",
			input: "1,3-5,7",
			want:  []int{1, 3, 4, 5, 7},
		},
		{
			name:  "with spaces around commas",
			input: "1, 3, 5",
			want:  []int{1, 3, 5},
		},
		{
			name:  "range in mixed input",
			input: "1,2-4",
			want:  []int{1, 2, 3, 4},
		},
		{
			name:  "deduplicates",
			input: "1,1,2,2,3",
			want:  []int{1, 2, 3},
		},
		{
			name:  "single range",
			input: "3-3",
			want:  []int{3},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid number",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "zero",
			input:   "0",
			wantErr: true,
		},
		{
			name:    "negative",
			input:   "-1",
			wantErr: true,
		},
		{
			name:    "reversed range",
			input:   "5-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNumbers(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNumbers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseNumbers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseNumberArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []int
		wantErr bool
	}{
		{
			name: "multiple args",
			args: []string{"1", "3", "5"},
			want: []int{1, 3, 5},
		},
		{
			name: "single arg with comma",
			args: []string{"1,3,5"},
			want: []int{1, 3, 5},
		},
		{
			name: "mixed",
			args: []string{"1", "3-5", "7"},
			want: []int{1, 3, 4, 5, 7},
		},
		{
			name:    "empty",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNumberArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNumberArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseNumberArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
