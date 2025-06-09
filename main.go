package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"

	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/term"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-sixel"
)

// - scan through all folders (and subfolders if specified) grabbing all supported image files
// - hash those image files as they came in and add them to a hashmap, collecting any collisions that it finds
// - convert those images to properly scaled sixel images
// when that's all done, display the main ui and program
// flip through all image duplicates with options to either do nothing, delete 1 of the images, or delete both (with an are you sure?)
// those marks get added to a list of actions that is only run when you go through all of the images or say that you're done and do them now (that should give you an option to either continue or quit)
// if you resize the terminal at all, it should wait until the size has stopped changing and then resize all the images accordingly
//

// stage 1:
// present the loading-setting up screen and count up the amount of
// files and images scanned

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

	go func() {
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
	}()

	go func() {
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
	}()

	out := make(chan []DupeSet, 1)
	go convImages(countSub, dupeSub, out)

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
	for _, path := range toDel {
		fmt.Printf("deleting file %s\n", path)
		err := os.Remove(path)
		if err != nil {
			fmt.Println(err)
		}
	}

	fmt.Printf("All done! Deleted %d files\n", len(toDel))
}

type DupeEntry struct {
	Path   string
	DupeOf string
}

type DupeSet struct {
	Paths    []string
	SixelImg []byte
	Pos      int
}

func convImages(countSub chan countMsg, dupeSub chan DupeEntry, out chan []DupeSet) {
	var t_w, t_h int

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
		height = 24
	}
	font_width, font_height := getFontCellSize()
	t_w = width * font_width
	t_h = (height - 7) * font_height

	dupes := make(map[string]DupeSet)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for range max(runtime.NumCPU()-3, 1) {
		wg.Add(1)
		go dupeRoutine(&wg, dupeSub, &mu, dupes, countSub, t_w, t_h)
	}

	wg.Wait()
	countSub <- AllDoneMsg
	close(countSub)
	var ret []DupeSet
	for _, set := range dupes {
		ret = append(ret, set)
	}
	out <- ret
	close(out)

}

func dupeRoutine(wg *sync.WaitGroup, dupeSub chan DupeEntry, mu *sync.Mutex, dupes map[string]DupeSet, countSub chan countMsg, width, height int) {
	defer wg.Done()
	for dupe := range dupeSub {
		mu.Lock()
		entry, ok := dupes[dupe.DupeOf]
		if !ok {
			entry.Paths = []string{dupe.DupeOf}
			dupes[dupe.DupeOf] = entry
		} else {
			entry.Paths = append(entry.Paths, dupe.Path)
		}
		mu.Unlock()

		if !ok {
			sixelImg := toSixelImg(dupe.DupeOf, width, height)
			countSub <- ImageConvertedCount

			mu.Lock()
			entry = dupes[dupe.DupeOf]
			entry.SixelImg = sixelImg
			entry.Paths = append(entry.Paths, dupe.Path)
			dupes[dupe.DupeOf] = entry
			mu.Unlock()
			continue
		}
		mu.Lock()
		entry = dupes[dupe.DupeOf]
		entry.Paths = append(entry.Paths, dupe.Path)
		dupes[dupe.DupeOf] = entry
		mu.Unlock()
	}
}

func toSixelImg(path string, maxWidth, maxHeight int) []byte {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	img, _, err := image.Decode(file)
	file.Close()
	if err != nil {
		panic(err)
	}
	img = resizeImageNearest(img, maxWidth, maxHeight)
	var buf bytes.Buffer
	err = sixel.NewEncoder(&buf).Encode(img)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func resizeImageNearest(src image.Image, maxWidth, maxHeight int) image.Image {
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	if srcWidth <= maxWidth && srcHeight <= maxHeight {
		return src
	}
	scaleX := float64(maxWidth) / float64(srcWidth)
	scaleY := float64(maxHeight) / float64(srcHeight)

	scale := min(scaleY, scaleX)

	newWidth := int(float64(srcWidth) * scale)
	newHeight := int(float64(srcHeight) * scale)
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	return dst
}

func getFontCellSize() (width, height int) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return 16, 8
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	fmt.Print("\033[16t")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('t')
	if err != nil {
		return 16, 8
	}

	parts := strings.Split(response, ";")
	if len(parts) >= 3 {
		height, _ = strconv.Atoi(parts[1])
		width, _ = strconv.Atoi(strings.TrimSuffix(parts[2], "t"))
	}

	return width, height
}

func checkErrAndExit(err error, shouldExit bool, exitMsg string) {
	if err != nil {
		panic(err)
	}
	if shouldExit {
		if exitMsg != "" {
			fmt.Println(exitMsg)
		}
		os.Exit(0)
	}
}

func checkTypeError(ok bool) {
	if !ok {
		panic("unknown type error")
	}
}
