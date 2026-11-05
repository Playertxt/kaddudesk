package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// rootEntries lists the drive letters for the file browser's top level.
func rootEntries() []fileEntry {
	var ents []fileEntry
	mask, _ := windows.GetLogicalDrives()
	for i := 0; i < 26; i++ {
		if mask&(1<<i) != 0 {
			d := fmt.Sprintf("%c:\\", 'A'+i)
			ents = append(ents, fileEntry{Name: d, Path: d, Dir: true})
		}
	}
	return ents
}
