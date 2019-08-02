// +build linux

package keyctl

import (
	"crypto/rand"
	"testing"
)

func TestSessionKeyring(t *testing.T) {

	token := make([]byte, 20)
	rand.Read(token)

	testname := "testname"
	keyring, err := SessionKeyring()
	if err != nil {
		t.Fatal(err)
	}
	_, err = keyring.Add(testname, token)
	if err != nil {
		t.Fatal(err)
	}
	key, err := keyring.Search(testname)
	if err != nil {
		t.Fatal(err)
	}
	data, err := key.Get()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(token) {
		t.Errorf("Expected data %v, but get %v", token, data)
	}
}

func TestUserKeyring(t *testing.T) {
	token := make([]byte, 20)
	rand.Read(token)

	testname := "testuser"

	userKeyring, err := UserKeyring()
	if err != nil {
		t.Fatal(err)
	}

	userKey, err := userKeyring.Add(testname, token)
	if err != nil {
		t.Fatal(err, userKey)
	}

	searchRet, err := userKeyring.Search(testname)
	if err != nil {
		t.Fatal(err)
	}
	if searchRet.Name != testname {
		t.Errorf("Expected data %v, but get %v", testname, searchRet.Name)
	}
}

func TestLink(t *testing.T) {
	token := make([]byte, 20)
	rand.Read(token)

	testname := "testlink"

	userKeyring, err := UserKeyring()
	if err != nil {
		t.Fatal(err)
	}

	sessionKeyring, err := SessionKeyring()
	if err != nil {
		t.Fatal(err)
	}

	key, err := sessionKeyring.Add(testname, token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = userKeyring.Search(testname)
	if err == nil {
		t.Fatalf("Expected error, but got key %v", testname)
	}
	ExpectedError := "required key not available"
	if err.Error() != ExpectedError {
		t.Fatal(err)
	}

	err = Link(userKeyring, key)
	if err != nil {
		t.Fatal(err)
	}
	_, err = userKeyring.Search(testname)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnlink(t *testing.T) {
	token := make([]byte, 20)
	rand.Read(token)

	testname := "testunlink"
	keyring, err := SessionKeyring()
	if err != nil {
		t.Fatal(err)
	}
	key, err := keyring.Add(testname, token)
	if err != nil {
		t.Fatal(err)
	}

	err = Unlink(keyring, key)
	if err != nil {
		t.Fatal(err)
	}

	_, err = keyring.Search(testname)
	ExpectedError := "required key not available"
	if err.Error() != ExpectedError {
		t.Fatal(err)
	}
}
