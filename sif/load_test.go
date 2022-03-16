package sif

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDefFile(t *testing.T) {
	for _, c := range []struct {
		name        string
		input       string
		environment []string
		runscript   []string
	}{
		{"Empty input", "", []string{}, []string{}},
		{
			name: "Basic smoke test",
			input: "Bootstrap: library\n" +
				"%environment\n" +
				"			export FOO=world\n" +
				"			export BAR=baz\n" +
				"%runscript\n" +
				`			echo "Hello $FOO"` + "\n" +
				"			sleep 5\n" +
				"%help\n" +
				"			Abandon all hope.\n",
			environment: []string{"export FOO=world", "export BAR=baz"},
			runscript:   []string{`echo "Hello $FOO"`, "sleep 5"},
		},
		{
			name: "Trailing section marker",
			input: "Bootstrap: library\n" +
				"%environment\n" +
				"			export FOO=world\n" +
				"%runscript",
			environment: []string{"export FOO=world"},
			runscript:   []string{},
		},
	} {
		env, rs, err := parseDefFile(bytes.NewReader([]byte(c.input)))
		require.NoError(t, err, c.name)
		assert.Equal(t, c.environment, env, c.name)
		assert.Equal(t, c.runscript, rs, c.name)
	}
}

func TestGenerateInjectedScript(t *testing.T) {
	res := generateInjectedScript([]string{"export FOO=world", "export BAR=baz"},
		[]string{`echo "Hello $FOO"`, "sleep 5"})
	assert.Equal(t, "#!/bin/bash\n"+
		"export FOO=world\n"+
		"export BAR=baz\n"+
		`echo "Hello $FOO"`+"\n"+
		"sleep 5\n", string(res))
}
