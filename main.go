package main

import (
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	tea "github.com/charmbracelet/bubbletea"
)

var recurFlag = flag.Bool("no-recurse", false, "do not enter sub-directors")

func main() {
	var dir string
	if len(os.Args) != 2 {
		temp, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		dir = temp
	} else {
		dir = os.Args[1]
	}

	flag.Parse()
	countSub := make(chan countMsg, 1000)
	hashSub := make(chan string, 1000)
	dupeSub := make(chan DupeEntry, 100)

	go scanRoutine(dir, countSub, hashSub)

	go hashRoutine(hashSub, countSub, dupeSub)

	out := make(chan []DupeSet, 1)
	go dupeRoutineMaster(countSub, dupeSub, out)

	m := newDiscoverModel(countSub, dir)
	p := tea.NewProgram(m)

	m, err := p.Run()
	d_m, ok := m.(d_model)
	checkTypeError(ok)
	checkErrAndExit(err, d_m.shouldExit, d_m.exitMsg)

	dupes := <-out
	if len(dupes) == 0 {
		fmt.Println("No duplicates found! Have a nice day!")
		os.Exit(0)
	}
	next := newMainModel(dupes)

	p = tea.NewProgram(next)
	m, err = p.Run()
	m_m, ok := m.(m_model)
	checkTypeError(ok)
	checkErrAndExit(err, m_m.shouldExit, m_m.exitMsg)

	toDel := m_m.toDelete
	var i int
	for _, path := range toDel {
		fmt.Printf("deleting file %s\n", path)
		err := os.Remove(path)
		if err != nil {
			fmt.Println(err)
			continue
		}
		i++
	}

	fmt.Printf("All done! Deleted %d files\n", i)
}

func hashRoutine(hashSub chan string, countSub chan countMsg, dupeSub chan DupeEntry) {
	hashMap := make(map[uint32]string)
	for name := range hashSub {
		file, err := os.Open(name)
		if err != nil {
			panic(err)
		}
		table := crc32.MakeTable(crc32.Castagnoli)
		hash := crc32.New(table)
		_, err = io.Copy(hash, file)
		if err != nil {
			panic(err)
		}
		file.Close()
		hashSum := hash.Sum32()
		countSub <- FileHashedCount
		dupeOf, ok := hashMap[hashSum]
		if ok {
			dupeSub <- DupeEntry{Path: name, DupeOf: dupeOf}
			countSub <- DuplicatesFoundCount
		} else {
			hashMap[hashSum] = name
		}
	}
	close(dupeSub)
}

func scanRoutine(dir string, countSub chan countMsg, hashSub chan string) {
	dirs := []string{dir}
	for {
		popped := dirs[0]
		dirs = dirs[1:]
		entries, err := os.ReadDir(popped)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			countSub <- FileFoundCount
			entryName := entry.Name()
			fullPath := path.Join(popped, entryName)
			switch path.Ext(entryName) {
			case ".jpg", ".jpeg", ".png", ".webp":
				hashSub <- fullPath
				countSub <- ImageFoundCount
			}
			if *recurFlag {
				continue
			}
			if entry.IsDir() {
				dirs = append(dirs, fullPath)
				countSub <- FolderScannedCount
			}
		}
		if len(dirs) == 0 {
			close(hashSub)
			break
		}
	}
}
