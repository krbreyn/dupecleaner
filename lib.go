package main

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/gif"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/mattn/go-sixel"
	"golang.org/x/image/draw"
	"golang.org/x/term"
)

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

type DupeEntry struct {
	Path   string
	DupeOf string
}

type DupeSet struct {
	Paths    []string
	SixelImg []byte
	Pos      int
}

func hashRoutine(hashSub chan string, countSub chan countMsg, dupeSub chan DupeEntry) {
	hashMap := make(map[uint32]string)
	table := crc32.MakeTable(crc32.Castagnoli)
	for name := range hashSub {
		file, err := os.Open(name)
		if err != nil {
			panic(err)
		}
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
			case ".jpg", ".jpeg", ".png", ".webp", ".gif":
				hashSub <- fullPath
				countSub <- ImageFoundCount
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

func dupeRoutineMaster(countSub chan countMsg, dupeSub chan DupeEntry, out chan []DupeSet) {
	var t_w, t_h int

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
		height = 24
	}
	font_width, font_height := getFontCellSize()
	t_w = width * font_width
	t_h = (height - 9) * font_height

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
			var sixelImg []byte
			if path.Ext(entry.Paths[0]) == ".gif" {
				sixelImg = toSixelImgGif(dupe.DupeOf, width, height)
			} else {
				sixelImg = toSixelImg(dupe.DupeOf, width, height)
			}
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
func toSixelImgGif(path string, maxWidth, maxHeight int) []byte {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	g, err := gif.DecodeAll(file)
	if err != nil {
		panic(err)
	}
	file.Close()
	img := resizeImageNearest(g.Image[0], maxWidth, maxHeight)
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
