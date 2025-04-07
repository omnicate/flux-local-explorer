package loader

import (
	"testing"
)

func Test_gitHttpsUrl(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "without user",
			input: "ssh://github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "with user",
			input: "ssh://git@github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "with .git",
			input: "ssh://git@github.com/omnicate/kubeconf.git",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "https",
			input: "https://github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "no scheme",
			input: "github.com/omnicate/kubeconf.git",
			want:  "https://github.com/omnicate/kubeconf.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitHttpsURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("gitSSHUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("gitSSHUrl() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gitSSHUrl(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "without user",
			input: "ssh://github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "with user",
			input: "ssh://git@github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "with .git",
			input: "ssh://git@github.com/omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "https",
			input: "https://github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "relative url",
			input: "ssh://git@github.com:omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "no scheme",
			input: "git@github.com:omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitSSHUrl(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("gitSSHUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("gitSSHUrl() got = %v, want %v", got, tt.want)
			}
		})
	}
}
