package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"os"
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

func dupeRoutineMaster(countSub chan countMsg, dupeSub chan DupeEntry, out chan []DupeSet) {
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
