package docker

import (
	"testing"

	"github.com/containers/image/docker/reference"
)

func Test_prepareMountQuery(t *testing.T) {

	parseNamedOrFail := func(name string) reference.Named {
		n, err := reference.ParseNamed(name)
		if err != nil {
			t.Fatalf("Error parsing name: %s", err)
		}
		return n
	}

	type args struct {
		mounts []reference.Named
		digest string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "single mount",
			args: args{
				mounts: []reference.Named{parseNamedOrFail("registry.com/foo")},
				digest: "somedigest",
			},
			want: "mount=somedigest&from=foo",
		},
		{
			name: "no mounts",
			args: args{
				mounts: []reference.Named{},
				digest: "somedigest",
			},
			want: "",
		},
		{
			name: "multiple mounts",
			args: args{
				mounts: []reference.Named{
					parseNamedOrFail("registry.com/foo"),
					parseNamedOrFail("registry.com/bar"),
					parseNamedOrFail("registry.com/baz/bat"),
				},
				digest: "somedigest",
			},
			want: "mount=somedigest&from=foo&from=bar&from=baz/bat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareMountQuery(tt.args.mounts, tt.args.digest); got != tt.want {
				t.Errorf("prepareMountQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
