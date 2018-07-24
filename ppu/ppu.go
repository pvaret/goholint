package ppu

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"tigris.fr/gameboy/fifo"
	"tigris.fr/gameboy/lcd"
	"tigris.fr/gameboy/memory"
	"tigris.fr/gameboy/ppu/states"
)

// ClockFactor representing the number of ticks taken by each step (base is 4).
var ClockFactor = 2

// LCDC flags. XXX: Move to subpackage lcdc for nicer namespacing?
const (
	// Bit 0 - BG/Window Display/Priority     (0=Off, 1=On)
	LCDCBGDisplay uint8 = 1 << iota
	// Bit 1 - OBJ (Sprite) Display Enable    (0=Off, 1=On)
	LCDCSpriteDisplayEnable
	// Bit 2 - OBJ (Sprite) Size              (0=8x8, 1=8x16)
	LCDCSpriteSize
	// Bit 3 - BG Tile Map Display Select     (0=9800-9BFF, 1=9C00-9FFF)
	LCDCBGTileMapDisplayeSelect
	// Bit 4 - BG & Window Tile Data Select   (0=8800-97FF, 1=8000-8FFF)
	LCDCBGWindowTileDataSelect
	// Bit 5 - Window Display Enable          (0=Off, 1=On)
	LCDCWindowDisplayEnable
	// Bit 6 - Window Tile Map Display Select (0=9800-9BFF, 1=9C00-9FFF)
	LCDCWindowTileMapDisplayeSelect
	// Bit 7 - LCD Display Enable             (0=Off, 1=On)
	LCDCDisplayEnable
)

// TileMapOffsets maps a Display Select flag to an address offset in VRAM.
var TileMapOffsets = [2]uint{0x9800, 0x9c00}

// PPU address space handling video RAM and display.
type PPU struct {
	*memory.MMU
	*fifo.FIFO
	Fetcher
	Cycle      int
	LCD        lcd.Display
	LCDC       uint8
	STAT       uint8
	SCY, SCX   uint8
	LY         uint8
	LYC        uint8
	WY, WX     uint8
	BGP        uint8
	OBP0, OBP1 uint8
	// TODO: DMA, address space to OAM, put in CPU

	ticks int
	state states.State

	oamIndex int
}

// New PPU instance.
func New(display lcd.Display) *PPU {
	fifo := fifo.New(16, 8)
	fetcher := Fetcher{fifo: fifo}
	p := PPU{MMU: memory.NewMMU([]memory.Addressable{}), FIFO: fifo, Fetcher: fetcher, LCD: display}
	p.Add(memory.Registers{
		0xff40: &p.LCDC,
		0xff41: &p.STAT,
		0xff42: &p.SCY,
		0xff43: &p.SCX,
		0xff44: &p.LY,
		0xff45: &p.LYC,
		0xff47: &p.BGP,
		0xff48: &p.OBP0,
		0xff49: &p.OBP1,
		0xff4a: &p.WY,
		0xff4b: &p.WX,
	})
	p.Add(memory.NewVRAM(0x8000, 0x2000)) // VRAM
	p.Add(memory.NewVRAM(0xfe00, 0xa0))   // OAM RAM (TODO: mapped OBJ struct)
	return &p
}

// Tick advances the CPU state one step.
func (p *PPU) Tick() {
	p.Cycle++
	p.ticks++
	if p.ticks < ClockFactor {
		return
	}

	// Reset tick counter and execute next state
	p.ticks = 0

	if p.LCDC&LCDCDisplayEnable == 0 {
		// Refresh window with "disabled screen" texture.
		p.LCD.Blank()
	}

	switch p.state {
	case states.OAMSearch:
		p.oamIndex++
		if p.oamIndex >= 40 {
			p.state = states.PixelTransfer
			return
		}

		// TODO

	case states.PixelTransfer:

	case states.HBlank:

	case states.VBlank:
	}
}

// Read a byte from VRAM/registers in the proper number of cycles.
func (p *PPU) Read(addr uint) uint8 {
	return p.MMU.Read(addr)
}

// Pop tries shifting a pixel out of the FIFO a,d returns the number of shifted pixels (0 or 1).
func (p *PPU) Pop() uint {
	if pixel, err := p.FIFO.Pop(); err == nil {
		p.LCD.Write(lcd.Pixel(pixel.(int)))
		return 1
	}
	return 0
}

// FetchTileNumber returns the index of the tile at current LCD coordinates in the background map. Takes one PPU Tick.
func (p *PPU) FetchTileNumber(x, y uint) uint8 {
	// XXX: Must this be computed for every pixel, or can we get away with doing it once per line?
	var bgMapOffset uint
	if (p.LCDC & LCDCBGTileMapDisplayeSelect) > 0 {
		bgMapOffset = 0x9c00
	} else {
		bgMapOffset = 0x9800
	}
	bgMapIndex := uint(p.SCX)/8 + x/8 + (y/8)*32
	return p.Read(bgMapOffset + bgMapIndex)
}

// Run PPU process cadenced by the same clock driving the CPU.
func (p *PPU) Run() {
	for {
		for ; p.LY < 144; p.LY++ {
			// New line unless VBlank
			// TODO: OAM search (20 clocks)

			// Pixel transfer until HBlank
			for x := uint(0); x < 160; {
				// Pixel Transfer (~43 clocks)
				// Just draw background for now. Enough for our purpose. TODO: Window & sprites

				// FIFO shifts out 2 pixels per fetcher read.
				x += p.Pop()
				x += p.Pop()
				y := uint(p.SCY + p.LY)
				tileNb := p.FetchTileNumber(x, y) // Tick()

				x += p.Pop()
				x += p.Pop()
				// Compute address of first byte of tile data to render.
				tileLine := uint(y % 8)
				var tileDataOffset uint
				if p.LCDC&LCDCBGWindowTileDataSelect > 0 {
					tileDataOffset = 0x8000 + uint(tileNb)*16
				} else {
					tileDataOffset = uint(0x9000 + int(tileNb)*16)
				}
				addr := tileDataOffset + tileLine*2
				lineLo := p.Read(addr) // Tick()

				x += p.Pop()
				x += p.Pop()
				lineHi := p.Read(addr + 1) // Tick()

				// Wait for FIFO to be ready to accept more data TODO: fill it now if there is room
				x += p.Pop()
				x += p.Pop()
				for bit := 7; bit >= 0; bit-- {
					pixel := (lineHi>>uint(bit)&1)<<1 | (lineLo >> uint(bit) & 1)
					p.Push(pixel)
				}
				p.Tick()
			}

			// TODO: HBlank (~51 clocks)
			p.LCD.HBlank()
			fmt.Println("HBLANK")
			//p.Ticks(51)
		}

		p.LCD.VBlank() // (114 clocks * 10)
		fmt.Println("VBLANK")
		for ; p.LY < 154; p.LY++ {
		}

		p.LY = 0
		// Anything else?
	}
}

// DumpTiles writes tiles from VRAM into a PNG file to test the decoder.
func (p *PPU) DumpTiles(addr, len uint) {

	// FIXME: handle native palettes
	palette := color.Palette{
		color.RGBA{0xff, 0xff, 0xff, 0xff},
		color.RGBA{0xaa, 0xaa, 0xaa, 0xff},
		color.RGBA{0x55, 0x55, 0x55, 0xff},
		color.RGBA{0x00, 0x00, 0x00, 0xff},
	}

	start := addr
	// Don't bother re-aligning tile lines yet, use an 8-pixels wide image.
	dump := image.NewPaletted(image.Rect(0, 0, 8, int(8*len)), palette)
	offset := 0
	for tile := 0; tile < int(len); tile++ {
		for line := 0; line < 8; line++ {
			pixels := p.Decode(start)
			for _, pixel := range pixels {
				dump.Pix[offset] = pixel
				offset++
			}
			start += 2 // 2 bytes per tile line
		}
	}

	f, err := os.Create("tiles-dump.png")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()
	w := bufio.NewWriter(f)
	png.Encode(w, dump)
	w.Flush()

	// Dump VRAM for checks
	//ioutil.WriteFile("vram-dump.bin", p.vram.Bytes, 0666)
}

// Decode reads 8 pixels from VRAM and returns them as an array of colors (aka palette indexes). TODO: Fetcher.
func (p *PPU) Decode(addr uint) (line []uint8) {
	lineLo := p.Read(addr)
	lineHi := p.Read(addr + 1)
	// TODO: push directly to FIFO
	line = make([]uint8, 0, 8)
	for bit := 7; bit >= 0; bit-- {
		pixel := (lineHi>>uint(bit)&1)<<1 | (lineLo >> uint(bit) & 1)
		line = append(line, pixel)
	}
	return line
}
