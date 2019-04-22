package ppu

import (
	"go.tigris.fr/gameboy/memory"
	"go.tigris.fr/gameboy/ppu/states"
)

// Fetcher reads tile data from VRAM and pushes pixels to PPU FIFO.
type Fetcher struct {
	Enabled         bool
	fifo            *FIFO
	vRAM            memory.Addressable
	ticks           int
	state, oldState states.State
	mapAddr         uint  // Start address of BG/Windows map row
	dataAddr        uint  // Start address of Sprite/BG tile data
	tileOffset      uint8 // X offset in the tile map row (will wrap around)
	tileLine        uint8 // Y offset (in pixels) in the tile
	signedID        bool

	tileID   uint8
	tileData [8]uint8

	sprite       Sprite // Stores X, Y and address in OAM
	spriteID     uint8
	spriteFlags  uint8
	spriteOffset uint8 // X offset for sprite (if not fully on screen)
	spriteLine   uint8 // Y offset (in pixels) in the sprite
	spriteData   [8]uint8
}

// Start fetching a line of pixels from the given tile in the given tilemap
// address space when Tick() is called.
func (f *Fetcher) Start(mapAddr, dataAddr uint, tileOffset, tileLine uint8, signedID bool) {
	f.mapAddr, f.dataAddr = mapAddr, dataAddr
	f.tileOffset, f.tileLine = tileOffset, tileLine
	f.signedID = signedID
	f.state = states.ReadTileID
	f.Enabled = true
	f.fifo.Clear()
}

// FetchSprite pauses the current fetching state to read sprite data and mix it
// in the pixel FIFO.
func (f *Fetcher) FetchSprite(sprite Sprite, spriteOffset, spriteLine uint8) {
	f.sprite = sprite
	f.spriteOffset, f.spriteLine = spriteOffset, spriteLine
	f.oldState = f.state
	f.state = states.ReadSpriteID
}

// Tick advances the fetcher's state machine one step.
func (f *Fetcher) Tick() {
	if !f.Enabled {
		return
	}
	f.ticks++
	if f.ticks < ClockFactor {
		return
	}

	// Reset tick counter and execute next state
	f.ticks = 0

	switch f.state {
	case states.ReadTileID:
		f.tileID = f.vRAM.Read(f.mapAddr + uint(f.tileOffset))
		f.state = states.ReadTileData0
		//logger.Printf("fetcher", "%04x: %02x\n", f.mapAddr+uint(f.tileOffset), f.tileID)

	case states.ReadTileData0:
		f.ReadTileLine(0, f.dataAddr, f.tileID, f.signedID, f.tileLine, 0, &f.tileData)
		f.state = states.ReadTileData1

	case states.ReadTileData1:
		f.ReadTileLine(1, f.dataAddr, f.tileID, f.signedID, f.tileLine, 0, &f.tileData)
		f.state = states.PushToFIFO

	case states.PushToFIFO:
		if f.fifo.Size() <= 8 {
			for i := 0; i < 8; i++ { // TODO: PixelFIFO directly handling [8]uint8
				f.fifo.Push(f.tileData[i])
			}
			f.tileOffset = (f.tileOffset + 1) % 32
			f.state = states.ReadTileID
		}
	case states.ReadSpriteID:
		f.spriteID = f.vRAM.Read(f.sprite.Address + 2) // We already read X&Y
		f.state = states.ReadSpriteFlags

	case states.ReadSpriteFlags:
		f.spriteFlags = f.vRAM.Read(f.sprite.Address + 3)
		f.state = states.ReadSpriteData0

	case states.ReadSpriteData0:
		// TODO: 16px high sprites
		f.ReadTileLine(0, 0x8000, f.spriteID, false, f.spriteLine, f.spriteFlags, &f.spriteData)
		f.state = states.ReadSpriteData1

	case states.ReadSpriteData1:
		f.ReadTileLine(1, 0x8000, f.spriteID, false, f.spriteLine, f.spriteFlags, &f.spriteData)
		f.state = states.MixInFIFO

	case states.MixInFIFO:
		if f.fifo.Size() < 8 {
			break
		}

		// Mix sprite pixels with FIFO, taking into account offset if sprite
		// is only partially displayed (i.e. entering screen from the left).
		// TODO: use f.spriteOffset
		for i := 0; i < 8; i++ {
			f.fifo.Mix(i, f.spriteData[i])
		}
		f.state = f.oldState
	}
}

// ReadTileLine updates internal pixel buffer with LSB or MSB tile line
// depending on current state.
func (f *Fetcher) ReadTileLine(bitPlane uint8, tileDataAddr uint, tileID uint8, signedID bool, tileLine uint8, flags uint8, data *[8]uint8) {
	// TODO: attributes, 16-pixel height
	var offset uint
	if signedID {
		offset = uint(int(tileDataAddr) + int(int8(tileID))*16)
	} else {
		offset = tileDataAddr + (uint(tileID) * 16)
	}
	if flags&SpriteFlipY != 0 {
		tileLine = 7 - tileLine // TODO: 16px height
	}
	addr := offset + (uint(tileLine) * 2)

	pixelData := f.vRAM.Read(addr + uint(bitPlane))
	for bitPos := 7; bitPos >= 0; bitPos-- {
		var pixelIndex uint
		if flags&SpriteFlipX != 0 {
			pixelIndex = uint(bitPos)
		} else {
			pixelIndex = 7 - uint(bitPos)
		}
		if bitPlane == 0 {
			// Least significant bit, replace previous value.
			data[pixelIndex] = (pixelData >> uint(bitPos)) & 1
		} else {
			// Most significant bit, update previous value.
			data[pixelIndex] |= ((pixelData >> uint(bitPos)) & 1) << 1
		}
	}
}
