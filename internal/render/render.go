package render

import (
	"bytes"
	"fmt"
	"image/color"
	"image/jpeg"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"

	"github.com/huugof/quote-cards/internal/static"
	"github.com/huugof/quote-cards/internal/util"
)

const (
	CardWidth       = 1200
	CardHeight      = 628
	CardPaddingX    = 150
	CardPaddingY    = 120
	QuoteFontMax    = 72.0
	QuoteFontMin    = 36.0
	QuoteFontStep   = 2.0
	QuoteLineHeight = 1.32
)

var (
	regularFont *truetype.Font
	faceCache   = struct {
		sync.Mutex
		faces map[float64]font.Face
	}{faces: map[float64]font.Face{}}
)

func init() {
	var err error
	regularFont, err = truetype.Parse(static.AtkinsonRegular)
	if err != nil {
		panic(fmt.Errorf("parse regular font: %w", err))
	}
}

func RenderQuoteCard(text string) ([]byte, error) {
	display := fmt.Sprintf("“%s”", util.NormalizeWhitespace(text))
	availableWidth := float64(CardWidth - CardPaddingX*2)
	availableHeight := float64(CardHeight - CardPaddingY*2)

	testCtx := gg.NewContext(CardWidth, CardHeight)

	var chosenSize float64
	var lines []string

	for size := QuoteFontMax; size >= QuoteFontMin; size -= QuoteFontStep {
		face := getFontFace(size)
		testCtx.SetFontFace(face)
		wrapped := testCtx.WordWrap(display, availableWidth)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		lineHeightPx := size * QuoteLineHeight
		height := float64(len(wrapped)) * lineHeightPx
		if height <= availableHeight {
			chosenSize = size
			lines = wrapped
			break
		}
	}

	if chosenSize == 0 {
		chosenSize = QuoteFontMin
		face := getFontFace(chosenSize)
		testCtx.SetFontFace(face)
		lines = testCtx.WordWrap(display, availableWidth)
		if len(lines) == 0 {
			lines = []string{""}
		}
	}

	dc := gg.NewContext(CardWidth, CardHeight)
	dc.SetColor(color.RGBA{R: 0xf7, G: 0xf4, B: 0xec, A: 0xff})
	dc.Clear()
	dc.SetColor(color.RGBA{R: 0x26, G: 0x21, B: 0x1a, A: 0xff})
	dc.SetFontFace(getFontFace(chosenSize))
	lineHeightPx := chosenSize * QuoteLineHeight
	startY := float64(CardHeight)/2 - (float64(len(lines))-1)*lineHeightPx/2

	for i, line := range lines {
		y := startY + float64(i)*lineHeightPx
		dc.DrawStringAnchored(line, float64(CardWidth)/2, y, 0.5, 0.5)
	}

	img := dc.Image()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getFontFace(size float64) font.Face {
	faceCache.Lock()
	defer faceCache.Unlock()
	if face, ok := faceCache.faces[size]; ok {
		return face
	}
	face := truetype.NewFace(regularFont, &truetype.Options{Size: size, DPI: 72, Hinting: font.HintingFull})
	faceCache.faces[size] = face
	return face
}
