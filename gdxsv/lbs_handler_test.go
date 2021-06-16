package main

import "testing"

func Test_isOldFlycastVersion(t *testing.T) {
	type args struct {
		userVersion string
	}
	tests := []struct {
		name            string
		requiredVersion string
		args            args
		want            bool
	}{
		{
			name:            "same version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v0.7.0"},
			want:            false,
		},
		{
			name:            "old version ng",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v0.6.9"},
			want:            true,
		},
		{
			name:            "new version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v9.9.9"},
			want:            false,
		},
		{
			name:            "gdxsv prefix version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "gdxsv-0.7.0"},
			want:            false,
		},
		{
			name:            "gdxsv prefix version ng",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "gdxsv-0.6.9"},
			want:            true,
		},
	}

	backup := requiredFlycastVersion
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredFlycastVersion = tt.requiredVersion
			if got := isOldFlycastVersion(tt.args.userVersion); got != tt.want {
				t.Errorf("isOldFlycastVersion() = %v, want %v", got, tt.want)
			}
		})
	}
	requiredFlycastVersion = backup
}
