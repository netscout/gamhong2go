package cpu

// Status register flag bits.
const (
	FlagC uint8 = 1 << iota // Carry
	FlagZ                   // Zero
	FlagI                   // Interrupt disable
	FlagD                   // Decimal mode
	FlagB                   // Break command
	FlagU                   // Unused (always 1)
	FlagV                   // Overflow
	FlagN                   // Negative
)

// Addressing modes for the 6502 instruction set.
type addrMode uint8

const (
	mImplied addrMode = iota
	mAccumulator
	mImmediate
	mZeroPage
	mZeroPageX
	mZeroPageY
	mAbsolute
	mAbsoluteX
	mAbsoluteY
	mIndirect
	mIndirectX // (Indirect,X) — indexed indirect
	mIndirectY // (Indirect),Y — indirect indexed
	mRelative
)

// Memory is the interface the CPU uses to read and write the outside world.
// In tests this is a flat 64 KB array; later it becomes the address bus.
type Memory interface {
	Read(addr uint16) uint8
	Write(addr uint16, val uint8)
}

// CPU holds the complete state of a MOS 6502 processor.
type CPU struct {
	A  uint8  // Accumulator
	X  uint8  // X index register
	Y  uint8  // Y index register
	SP uint8  // Stack pointer (offset within page $01)
	PC uint16 // Program counter
	P  uint8  // Processor status flags

	Mem    Memory // Attached memory / bus
	Cycles uint64 // Total elapsed cycles

	pageCrossed bool // Set by resolve() when indexing crosses a page
	extraCycles int  // Extra cycles added by branches
}

// NewCPU returns a CPU wired to the given memory, in post-reset state.
func NewCPU(mem Memory) *CPU {
	c := &CPU{
		SP:  0xFD,
		P:   FlagU | FlagI,
		Mem: mem,
	}
	return c
}

// Reset performs a hardware reset: loads PC from the reset vector ($FFFC)
// and initialises the stack pointer and status register.
func (c *CPU) Reset() {
	c.PC = c.read16(0xFFFC)
	c.SP = 0xFD
	c.P = FlagU | FlagI
}

// Step executes one instruction and returns the number of cycles it consumed.
func (c *CPU) Step() int {
	opcode := c.read(c.PC)
	c.PC++
	c.pageCrossed = false
	c.extraCycles = 0

	inst := &opcodeTable[opcode]
	inst.exec(c, inst.mode)

	cycles := int(inst.cycles)
	if c.pageCrossed && inst.pagePenalty {
		cycles++
	}
	cycles += c.extraCycles
	c.Cycles += uint64(cycles)
	return cycles
}

// ---------------------------------------------------------------------------
// Addressing mode resolution
// ---------------------------------------------------------------------------

// resolve reads any operand bytes after the opcode and returns the effective
// address. It advances PC past the operand and sets c.pageCrossed when an
// indexed mode crosses a 256-byte page boundary.
func (c *CPU) resolve(mode addrMode) uint16 {
	switch mode {
	case mImmediate:
		addr := c.PC
		c.PC++
		return addr

	case mZeroPage:
		addr := uint16(c.read(c.PC))
		c.PC++
		return addr

	case mZeroPageX:
		addr := uint16(c.read(c.PC) + c.X) // wraps within zero page(convert to 16bit after adding 8bit + 8bit which causes overflow)
		c.PC++
		return addr

	case mZeroPageY:
		addr := uint16(c.read(c.PC) + c.Y)
		c.PC++
		return addr

	case mAbsolute:
		addr := c.read16(c.PC)
		c.PC += 2
		return addr

	case mAbsoluteX:
		base := c.read16(c.PC)
		c.PC += 2
		addr := base + uint16(c.X)
		c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
		return addr

	case mAbsoluteY:
		base := c.read16(c.PC)
		c.PC += 2
		addr := base + uint16(c.Y)
		c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
		return addr

	case mIndirect:
		ptr := c.read16(c.PC)
		c.PC += 2
		return c.read16Wrap(ptr) // reproduces the NMOS page-boundary bug

	case mIndirectX:
		base := c.read(c.PC)
		c.PC++
		ptr := uint16(base + c.X) // wraps within zero page
		lo := uint16(c.read(ptr))
		hi := uint16(c.read((ptr + 1) & 0x00FF))
		return hi<<8 | lo

	case mIndirectY:
		ptr := uint16(c.read(c.PC))
		c.PC++
		lo := uint16(c.read(ptr))
		hi := uint16(c.read((ptr + 1) & 0x00FF))
		base := hi<<8 | lo
		addr := base + uint16(c.Y)
		c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
		return addr

	case mRelative:
		offset := uint16(c.read(c.PC))
		c.PC++
		if offset&0x80 != 0 {
			offset |= 0xFF00 // sign-extend
		}
		return c.PC + offset

	default: // implied, accumulator
		return 0
	}
}

// ---------------------------------------------------------------------------
// Memory helpers
// ---------------------------------------------------------------------------

func (c *CPU) read(addr uint16) uint8 {
	return c.Mem.Read(addr)
}

func (c *CPU) write(addr uint16, val uint8) {
	c.Mem.Write(addr, val)
}

// read16 reads a little-endian 16-bit value from two consecutive bytes.
func (c *CPU) read16(addr uint16) uint16 {
	lo := uint16(c.read(addr))
	hi := uint16(c.read(addr + 1))
	return hi<<8 | lo
}

// read16Wrap reproduces the NMOS 6502 bug where JMP ($xxFF) fetches the
// high byte from $xx00 instead of $(xx+1)00.
func (c *CPU) read16Wrap(addr uint16) uint16 {
	lo := uint16(c.read(addr))
	hiAddr := (addr & 0xFF00) | uint16(uint8(addr)+1)
	hi := uint16(c.read(hiAddr))
	return hi<<8 | lo
}

// ---------------------------------------------------------------------------
// Stack helpers — the stack lives in page $01 ($0100–$01FF)
// ---------------------------------------------------------------------------

func (c *CPU) push(val uint8) {
	c.write(0x0100|uint16(c.SP), val) // Stack pointer's high byte is always 0x01(hardwired), so SP only need low byte to calculate the address
	c.SP--
}

func (c *CPU) pull() uint8 {
	c.SP++
	return c.read(0x0100 | uint16(c.SP))
}

func (c *CPU) push16(val uint16) {
	c.push(uint8(val >> 8))
	c.push(uint8(val))
}

func (c *CPU) pull16() uint16 {
	lo := uint16(c.pull())
	hi := uint16(c.pull())
	return hi<<8 | lo
}

// ---------------------------------------------------------------------------
// Flag helpers
// ---------------------------------------------------------------------------

func (c *CPU) setFlag(flag uint8, on bool) {
	if on {
		c.P |= flag
	} else {
		c.P &^= flag
	}
}

func (c *CPU) getFlag(flag uint8) bool {
	return c.P&flag != 0
}

// setZN sets the Zero and Negative flags based on val.
func (c *CPU) setZN(val uint8) {
	c.setFlag(FlagZ, val == 0)
	c.setFlag(FlagN, val&0x80 != 0)
}

// ---------------------------------------------------------------------------
// Branch helper
// ---------------------------------------------------------------------------

// branch conditionally jumps to the target address resolved from a relative
// operand. Adds 1 extra cycle if taken, 2 if taken and page-crossing.
func (c *CPU) branch(mode addrMode, condition bool) {
	target := c.resolve(mode)
	if condition {
		c.extraCycles++
		targetOnSamePage := (c.PC & 0xFF00) == (target & 0xFF00)
		if !targetOnSamePage {
			c.extraCycles++
		}
		c.PC = target
	}
}
