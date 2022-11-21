package reference

import (
	"regexp"
	"strings"
	"testing"
)

type regexpMatch struct {
	input string
	match bool
	subs  []string
}

func checkRegexp(t *testing.T, r *regexp.Regexp, m regexpMatch) {
	t.Helper()
	matches := r.FindStringSubmatch(m.input)
	if m.match && matches != nil {
		if len(matches) != (r.NumSubexp()+1) || matches[0] != m.input {
			t.Fatalf("Bad match result %#v for %q", matches, m.input)
		}
		if len(matches) < (len(m.subs) + 1) {
			t.Errorf("Expected %d sub matches, only have %d for %q", len(m.subs), len(matches)-1, m.input)
		}
		for i := range m.subs {
			if m.subs[i] != matches[i+1] {
				t.Errorf("Unexpected submatch %d: %q, expected %q for %q", i+1, matches[i+1], m.subs[i], m.input)
			}
		}
	} else if m.match {
		t.Errorf("Expected match for %q", m.input)
	} else if matches != nil {
		t.Errorf("Unexpected match for %q", m.input)
	}
}

func TestDomainRegexp(t *testing.T) {
	t.Parallel()
	hostcases := []struct {
		input string
		match bool
	}{
		{
			input: "test.com",
			match: true,
		},
		{
			input: "test.com:10304",
			match: true,
		},
		{
			input: "test.com:http",
			match: false,
		},
		{
			input: "localhost",
			match: true,
		},
		{
			input: "localhost:8080",
			match: true,
		},
		{
			input: "a",
			match: true,
		},
		{
			input: "a.b",
			match: true,
		},
		{
			input: "ab.cd.com",
			match: true,
		},
		{
			input: "a-b.com",
			match: true,
		},
		{
			input: "-ab.com",
			match: false,
		},
		{
			input: "ab-.com",
			match: false,
		},
		{
			input: "ab.c-om",
			match: true,
		},
		{
			input: "ab.-com",
			match: false,
		},
		{
			input: "ab.com-",
			match: false,
		},
		{
			input: "0101.com",
			match: true, // TODO(dmcgowan): valid if this should be allowed
		},
		{
			input: "001a.com",
			match: true,
		},
		{
			input: "b.gbc.io:443",
			match: true,
		},
		{
			input: "b.gbc.io",
			match: true,
		},
		{
			input: "xn--n3h.com", // ☃.com in punycode
			match: true,
		},
		{
			input: "Asdf.com", // uppercase character
			match: true,
		},
		{
			input: "192.168.1.1:75050", // ipv4
			match: true,
		},
		{
			input: "192.168.1.1:750050", // port with more than 5 digits, it will fail on validation
			match: true,
		},
		{
			input: "[fd00:1:2::3]:75050", // ipv6 compressed
			match: true,
		},
		{
			input: "[fd00:1:2::3]75050", // ipv6 wrong port separator
			match: false,
		},
		{
			input: "[fd00:1:2::3]::75050", // ipv6 wrong port separator
			match: false,
		},
		{
			input: "[fd00:1:2::3%eth0]:75050", // ipv6 with zone
			match: false,
		},
		{
			input: "[fd00123123123]:75050", // ipv6 wrong format, will fail in validation
			match: true,
		},
		{
			input: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:75050", // ipv6 long format
			match: true,
		},
		{
			input: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:750505", // ipv6 long format and invalid port, it will fail in validation
			match: true,
		},
		{
			input: "fd00:1:2::3:75050", // bad ipv6 without square brackets
			match: false,
		},
	}
	r := regexp.MustCompile(`^` + DomainRegexp.String() + `$`)
	for _, tc := range hostcases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			match := r.MatchString(tc.input)
			if match != tc.match {
				t.Errorf("Expected match=%t, got %t", tc.match, match)
			}
		})
	}
}

func TestFullNameRegexp(t *testing.T) {
	t.Parallel()
	if anchoredNameRegexp.NumSubexp() != 2 {
		t.Fatalf("anchored name regexp should have two submatches: %v, %v != 2",
			anchoredNameRegexp, anchoredNameRegexp.NumSubexp())
	}

	testcases := []regexpMatch{
		{
			input: "",
			match: false,
		},
		{
			input: "short",
			match: true,
			subs:  []string{"", "short"},
		},
		{
			input: "simple/name",
			match: true,
			subs:  []string{"simple", "name"},
		},
		{
			input: "library/ubuntu",
			match: true,
			subs:  []string{"library", "ubuntu"},
		},
		{
			input: "docker/stevvooe/app",
			match: true,
			subs:  []string{"docker", "stevvooe/app"},
		},
		{
			input: "aa/aa/aa/aa/aa/aa/aa/aa/aa/bb/bb/bb/bb/bb/bb",
			match: true,
			subs:  []string{"aa", "aa/aa/aa/aa/aa/aa/aa/aa/bb/bb/bb/bb/bb/bb"},
		},
		{
			input: "aa/aa/bb/bb/bb",
			match: true,
			subs:  []string{"aa", "aa/bb/bb/bb"},
		},
		{
			input: "a/a/a/a",
			match: true,
			subs:  []string{"a", "a/a/a"},
		},
		{
			input: "a/a/a/a/",
			match: false,
		},
		{
			input: "a//a/a",
			match: false,
		},
		{
			input: "a",
			match: true,
			subs:  []string{"", "a"},
		},
		{
			input: "a/aa",
			match: true,
			subs:  []string{"a", "aa"},
		},
		{
			input: "a/aa/a",
			match: true,
			subs:  []string{"a", "aa/a"},
		},
		{
			input: "foo.com",
			match: true,
			subs:  []string{"", "foo.com"},
		},
		{
			input: "foo.com/",
			match: false,
		},
		{
			input: "foo.com:8080/bar",
			match: true,
			subs:  []string{"foo.com:8080", "bar"},
		},
		{
			input: "foo.com:http/bar",
			match: false,
		},
		{
			input: "foo.com/bar",
			match: true,
			subs:  []string{"foo.com", "bar"},
		},
		{
			input: "foo.com/bar/baz",
			match: true,
			subs:  []string{"foo.com", "bar/baz"},
		},
		{
			input: "localhost:8080/bar",
			match: true,
			subs:  []string{"localhost:8080", "bar"},
		},
		{
			input: "sub-dom1.foo.com/bar/baz/quux",
			match: true,
			subs:  []string{"sub-dom1.foo.com", "bar/baz/quux"},
		},
		{
			input: "blog.foo.com/bar/baz",
			match: true,
			subs:  []string{"blog.foo.com", "bar/baz"},
		},
		{
			input: "a^a",
			match: false,
		},
		{
			input: "aa/asdf$$^/aa",
			match: false,
		},
		{
			input: "asdf$$^/aa",
			match: false,
		},
		{
			input: "aa-a/a",
			match: true,
			subs:  []string{"aa-a", "a"},
		},
		{
			input: strings.Repeat("a/", 128) + "a",
			match: true,
			subs:  []string{"a", strings.Repeat("a/", 127) + "a"},
		},
		{
			input: "a-/a/a/a",
			match: false,
		},
		{
			input: "foo.com/a-/a/a",
			match: false,
		},
		{
			input: "-foo/bar",
			match: false,
		},
		{
			input: "foo/bar-",
			match: false,
		},
		{
			input: "foo-/bar",
			match: false,
		},
		{
			input: "foo/-bar",
			match: false,
		},
		{
			input: "_foo/bar",
			match: false,
		},
		{
			input: "foo_bar",
			match: true,
			subs:  []string{"", "foo_bar"},
		},
		{
			input: "foo_bar.com",
			match: true,
			subs:  []string{"", "foo_bar.com"},
		},
		{
			input: "foo_bar.com:8080",
			match: false,
		},
		{
			input: "foo_bar.com:8080/app",
			match: false,
		},
		{
			input: "foo.com/foo_bar",
			match: true,
			subs:  []string{"foo.com", "foo_bar"},
		},
		{
			input: "____/____",
			match: false,
		},
		{
			input: "_docker/_docker",
			match: false,
		},
		{
			input: "docker_/docker_",
			match: false,
		},
		{
			input: "b.gcr.io/test.example.com/my-app",
			match: true,
			subs:  []string{"b.gcr.io", "test.example.com/my-app"},
		},
		{
			input: "xn--n3h.com/myimage", // ☃.com in punycode
			match: true,
			subs:  []string{"xn--n3h.com", "myimage"},
		},
		{
			input: "xn--7o8h.com/myimage", // 🐳.com in punycode
			match: true,
			subs:  []string{"xn--7o8h.com", "myimage"},
		},
		{
			input: "example.com/xn--7o8h.com/myimage", // 🐳.com in punycode
			match: true,
			subs:  []string{"example.com", "xn--7o8h.com/myimage"},
		},
		{
			input: "example.com/some_separator__underscore/myimage",
			match: true,
			subs:  []string{"example.com", "some_separator__underscore/myimage"},
		},
		{
			input: "example.com/__underscore/myimage",
			match: false,
		},
		{
			input: "example.com/..dots/myimage",
			match: false,
		},
		{
			input: "example.com/.dots/myimage",
			match: false,
		},
		{
			input: "example.com/nodouble..dots/myimage",
			match: false,
		},
		{
			input: "example.com/nodouble..dots/myimage",
			match: false,
		},
		{
			input: "docker./docker",
			match: false,
		},
		{
			input: ".docker/docker",
			match: false,
		},
		{
			input: "docker-/docker",
			match: false,
		},
		{
			input: "-docker/docker",
			match: false,
		},
		{
			input: "do..cker/docker",
			match: false,
		},
		{
			input: "do__cker:8080/docker",
			match: false,
		},
		{
			input: "do__cker/docker",
			match: true,
			subs:  []string{"", "do__cker/docker"},
		},
		{
			input: "b.gcr.io/test.example.com/my-app",
			match: true,
			subs:  []string{"b.gcr.io", "test.example.com/my-app"},
		},
		{
			input: "registry.io/foo/project--id.module--name.ver---sion--name",
			match: true,
			subs:  []string{"registry.io", "foo/project--id.module--name.ver---sion--name"},
		},
		{
			input: "Asdf.com/foo/bar", // uppercase character in hostname
			match: true,
		},
		{
			input: "Foo/FarB", // uppercase characters in remote name
			match: false,
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			checkRegexp(t, anchoredNameRegexp, tc)
		})
	}
}

func TestReferenceRegexp(t *testing.T) {
	t.Parallel()
	if ReferenceRegexp.NumSubexp() != 3 {
		t.Fatalf("anchored name regexp should have three submatches: %v, %v != 3",
			ReferenceRegexp, ReferenceRegexp.NumSubexp())
	}

	testcases := []regexpMatch{
		{
			input: "registry.com:8080/myapp:tag",
			match: true,
			subs:  []string{"registry.com:8080/myapp", "tag", ""},
		},
		{
			input: "registry.com:8080/myapp@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: true,
			subs:  []string{"registry.com:8080/myapp", "", "sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912"},
		},
		{
			input: "registry.com:8080/myapp:tag2@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: true,
			subs:  []string{"registry.com:8080/myapp", "tag2", "sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912"},
		},
		{
			input: "registry.com:8080/myapp@sha256:badbadbadbad",
			match: false,
		},
		{
			input: "registry.com:8080/myapp:invalid~tag",
			match: false,
		},
		{
			input: "bad_hostname.com:8080/myapp:tag",
			match: false,
		},
		{
			input:// localhost treated as name, missing tag with 8080 as tag
			"localhost:8080@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: true,
			subs:  []string{"localhost", "8080", "sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912"},
		},
		{
			input: "localhost:8080/name@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: true,
			subs:  []string{"localhost:8080/name", "", "sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912"},
		},
		{
			input: "localhost:http/name@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: false,
		},
		{
			// localhost will be treated as an image name without a host
			input: "localhost@sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
			match: true,
			subs:  []string{"localhost", "", "sha256:be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912"},
		},
		{
			input: "registry.com:8080/myapp@bad",
			match: false,
		},
		{
			input: "registry.com:8080/myapp@2bad",
			match: false, // TODO(dmcgowan): Support this as valid
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			checkRegexp(t, ReferenceRegexp, tc)
		})
	}
}

func TestIdentifierRegexp(t *testing.T) {
	t.Parallel()
	fullCases := []struct {
		input string
		match bool
	}{
		{
			input: "da304e823d8ca2b9d863a3c897baeb852ba21ea9a9f1414736394ae7fcaf9821",
			match: true,
		},
		{
			input: "7EC43B381E5AEFE6E04EFB0B3F0693FF2A4A50652D64AEC573905F2DB5889A1C",
			match: false,
		},
		{
			input: "da304e823d8ca2b9d863a3c897baeb852ba21ea9a9f1414736394ae7fcaf",
			match: false,
		},
		{
			input: "sha256:da304e823d8ca2b9d863a3c897baeb852ba21ea9a9f1414736394ae7fcaf9821",
			match: false,
		},
		{
			input: "da304e823d8ca2b9d863a3c897baeb852ba21ea9a9f1414736394ae7fcaf98218482",
			match: false,
		},
	}
	for _, tc := range fullCases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			match := anchoredIdentifierRegexp.MatchString(tc.input)
			if match != tc.match {
				t.Errorf("Expected match=%t, got %t", tc.match, match)
			}
		})
	}
}
