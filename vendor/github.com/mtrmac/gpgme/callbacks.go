package gpgme

import (
	"sync"
)

var callbacks struct {
	sync.Mutex
	m map[int]interface{}
	c int
}

func callbackAdd(v interface{}) int {
	callbacks.Lock()
	defer callbacks.Unlock()
	if callbacks.m == nil {
		callbacks.m = make(map[int]interface{})
	}
	callbacks.c++
	ret := callbacks.c
	callbacks.m[ret] = v
	return ret
}

func callbackLookup(c int) interface{} {
	callbacks.Lock()
	defer callbacks.Unlock()
	ret := callbacks.m[c]
	if ret == nil {
		panic("callback pointer not found")
	}
	return ret
}

func callbackDelete(c int) {
	callbacks.Lock()
	defer callbacks.Unlock()
	if callbacks.m[c] == nil {
		panic("callback pointer not found")
	}
	delete(callbacks.m, c)
}
