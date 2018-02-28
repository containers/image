package docker

import (
	"testing"

	"github.com/containers/image/docker/reference"
)

func parseNamedOrFail(t *testing.T, name string) reference.Named {
	n, err := reference.ParseNamed(name)
	if err != nil {
		t.Fatalf("Error parsing name: %s", err)
	}
	return n
}

func Test_prepareMountQuery(t *testing.T) {

	type args struct {
		mount  *reference.Named
		digest string
	}
	ref := parseNamedOrFail(t, "registry.com/foo")
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "single mount",
			args: args{
				mount:  &ref,
				digest: "somedigest",
			},
			want: "mount=somedigest&from=foo",
		},
		{
			name: "no mounts",
			args: args{
				mount:  nil,
				digest: "somedigest",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareMountQuery(tt.args.mount, tt.args.digest); got != tt.want {
				t.Errorf("prepareMountQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateDockerMount(t *testing.T) {
	type args struct {
		ref   reference.Named
		mount reference.Named
	}
	dst := parseNamedOrFail(t, "someregistry.com/foo/bar:baz")
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "not name only",
			args: args{
				ref:   dst,
				mount: parseNamedOrFail(t, "someregistry.com/foo/baz:latest"),
			},
			wantErr: true,
		},
		{
			name: "not same dest",
			args: args{
				ref:   dst,
				mount: parseNamedOrFail(t, "someotherregistry.com/foo/baz"),
			},
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				ref:   dst,
				mount: parseNamedOrFail(t, "someregistry.com/foo/baz"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateDockerMount(tt.args.ref, tt.args.mount); (err != nil) != tt.wantErr {
				t.Errorf("validateDockerMount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
