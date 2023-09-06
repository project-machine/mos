package main

import (
	"os"
	"regexp"
)

var uuidMatch = regexp.MustCompile("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

func IsDir(d string) bool {
	s, err := os.Stat(d)
	if err != nil {
		return false
	}
	return s.IsDir()
}

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
