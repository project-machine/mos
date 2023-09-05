package printdirtree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

const (
	MiddleSym = "├──"
	ColumnSym = "│"
	LastSym   = "└──"
	FirstSym  = MiddleSym
)

func printDir(path, prevpath string, level, index, total int) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	numEntries := len(entries)
	for idx, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		padding := ""
		if level == 0 {
			padding = FirstSym
			if idx == (numEntries - 1) { // last one
				padding = LastSym
			}
		} else {
			// level 1 or more
			sym := ColumnSym
			if index == (total - 1) {
				sym = " "
			}
			padding = fmt.Sprintf("%s%s%s", sym, strings.Repeat(" ", (level*4)-1), FirstSym)
			if idx == (numEntries-1) || entry.IsDir() { // will recurse
				padding = fmt.Sprintf("%s%s%s", sym, strings.Repeat(" ", (level*4)-1), LastSym)
			}
		}
		if entryPath != prevpath {
			entryText := entry.Name()
			if entry.IsDir() {
				entryText = color.BlueString("%s", entryText)
			}
			fmt.Printf("%s %s\n", padding, entryText)
		}
		if entry.IsDir() {
			newLevel := level + 1
			return printDir(entryPath, path, newLevel, index, total)
		}
	}
	return nil
}

func PrintDirs(topDir string, dirList []string) error {
	fmt.Printf("%s\n", color.BlueString(topDir))
	for idx, dir := range dirList {
		subDir := filepath.Join(topDir, dir)
		padding := FirstSym
		if idx == (len(dirList) - 1) {
			padding = LastSym
		}
		fmt.Printf("%s %s\n", padding, color.BlueString(dir))
		if err := printDir(subDir, "", 1, idx, len(dirList)); err != nil {
			return err
		}
	}
	return nil
}
