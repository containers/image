///usr/bin/true; exec /usr/bin/env go run "$0" "$@"

package main

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
)

type userPasswordPair struct {
	User, Password string
}

type serverUserSecretTriple struct {
	ServerURL, Username, Secret string
}

func load() (map[string]userPasswordPair, error) {
	credentials := make(map[string]userPasswordPair)
	fname, ok := os.LookupEnv("CRED_HELPER_STORE_FILE")
	if !ok {
		return credentials, fmt.Errorf("the CRED_HELPER_STORE_FILE envvar not set")
	}
	f, err := os.OpenFile(fname, os.O_RDONLY, 0644)
	if err != nil {
		return credentials, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	err = dec.Decode(&credentials)
	return credentials, err
}

func store(credentials map[string]userPasswordPair) error {
	fname, ok := os.LookupEnv("CRED_HELPER_STORE_FILE")
	if !ok {
		return fmt.Errorf("the CRED_HELPER_STORE_FILE envvar not set")
	}
	f, err := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(&credentials)

}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	var se syscall.Errno
	if ok := errors.As(err, &se); ok {
		os.Exit(int(se))
	}
	os.Exit(-1)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "exactly one argument expected\n")
		os.Exit(int(syscall.EINVAL))
	}

	credentials, err := load()
	if err != nil {
		fatal(err)
	}

	switch os.Args[1] {
	case "list":
		outData := make(map[string]string)
		for server, up := range credentials {
			outData[server] = up.User
		}
		enc := json.NewEncoder(os.Stdout)
		err = enc.Encode(&outData)
		if err != nil {
			fatal(err)
		}
	case "get":
		inData, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		serverURL := string(inData)
		serverURL = strings.Trim(serverURL, "\r\n")
		up := credentials[serverURL]
		outData := serverUserSecretTriple{ServerURL: serverURL, Username: up.User, Secret: up.Password}
		enc := json.NewEncoder(os.Stdout)
		if err = enc.Encode(outData); err != nil {
			fatal(err)
		}
	case "erase":
		inData, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		serverURL := string(inData)
		serverURL = strings.Trim(serverURL, "\r\n")
		delete(credentials, serverURL)
		err = store(credentials)
		if err != nil {
			fatal(err)
		}
	case "store":
		inData := serverUserSecretTriple{}
		enc := json.NewDecoder(os.Stdin)
		err = enc.Decode(&inData)
		if err != nil {
			fatal(err)
		}
		credentials[inData.ServerURL] = userPasswordPair{User: inData.Username, Password: inData.Secret}
		err = store(credentials)
		if err != nil {
			fatal(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown sub-command %s\n", os.Args[1])
		os.Exit(int(syscall.EINVAL))
	}
}
