package main

import (
	"flag"
	"fmt"
	"os"

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
