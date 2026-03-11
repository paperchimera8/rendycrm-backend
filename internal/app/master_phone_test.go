package app

import "testing"

func TestNormalizeMasterPhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plus seven", input: "+7 (999) 111-22-33", want: "79991112233"},
		{name: "leading eight", input: "8 999 111 22 33", want: "79991112233"},
		{name: "ten digits", input: "9991112233", want: "79991112233"},
		{name: "invalid", input: "12345", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMasterPhone(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeMasterPhone: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
