package cpu

// opHandler is the signature for every instruction implementation.
type opHandler func(c *CPU, mode addrMode)

// opEntry describes a single opcode: its handler, addressing mode, base
// cycle count, and whether a page-crossing penalty applies.
type opEntry struct {
	exec        opHandler
	mode        addrMode
	cycles      uint8
	pagePenalty bool // +1 cycle when indexing crosses a page boundary
}

// opcodeTable is the full 256-entry dispatch table, populated in init().
var opcodeTable [256]opEntry

func init() {
	// Start with every slot as a NOP (handles illegal opcodes gracefully).
	for i := range opcodeTable {
		opcodeTable[i] = opEntry{opNOP, mImplied, 2, false}
	}

	// -----------------------------------------------------------------------
	// Load / Store
	// -----------------------------------------------------------------------
	set(0xA9, opLDA, mImmediate, 2, false)
	set(0xA5, opLDA, mZeroPage, 3, false)
	set(0xB5, opLDA, mZeroPageX, 4, false)
	set(0xAD, opLDA, mAbsolute, 4, false)
	set(0xBD, opLDA, mAbsoluteX, 4, true)
	set(0xB9, opLDA, mAbsoluteY, 4, true)
	set(0xA1, opLDA, mIndirectX, 6, false)
	set(0xB1, opLDA, mIndirectY, 5, true)

	set(0xA2, opLDX, mImmediate, 2, false)
	set(0xA6, opLDX, mZeroPage, 3, false)
	set(0xB6, opLDX, mZeroPageY, 4, false)
	set(0xAE, opLDX, mAbsolute, 4, false)
	set(0xBE, opLDX, mAbsoluteY, 4, true)

	set(0xA0, opLDY, mImmediate, 2, false)
	set(0xA4, opLDY, mZeroPage, 3, false)
	set(0xB4, opLDY, mZeroPageX, 4, false)
	set(0xAC, opLDY, mAbsolute, 4, false)
	set(0xBC, opLDY, mAbsoluteX, 4, true)

	set(0x85, opSTA, mZeroPage, 3, false)
	set(0x95, opSTA, mZeroPageX, 4, false)
	set(0x8D, opSTA, mAbsolute, 4, false)
	set(0x9D, opSTA, mAbsoluteX, 5, false)
	set(0x99, opSTA, mAbsoluteY, 5, false)
	set(0x81, opSTA, mIndirectX, 6, false)
	set(0x91, opSTA, mIndirectY, 6, false)

	set(0x86, opSTX, mZeroPage, 3, false)
	set(0x96, opSTX, mZeroPageY, 4, false)
	set(0x8E, opSTX, mAbsolute, 4, false)

	set(0x84, opSTY, mZeroPage, 3, false)
	set(0x94, opSTY, mZeroPageX, 4, false)
	set(0x8C, opSTY, mAbsolute, 4, false)

	// -----------------------------------------------------------------------
	// Register transfers
	// -----------------------------------------------------------------------
	set(0xAA, opTAX, mImplied, 2, false)
	set(0xA8, opTAY, mImplied, 2, false)
	set(0x8A, opTXA, mImplied, 2, false)
	set(0x98, opTYA, mImplied, 2, false)
	set(0xBA, opTSX, mImplied, 2, false)
	set(0x9A, opTXS, mImplied, 2, false)

	// -----------------------------------------------------------------------
	// Stack
	// -----------------------------------------------------------------------
	set(0x48, opPHA, mImplied, 3, false)
	set(0x08, opPHP, mImplied, 3, false)
	set(0x68, opPLA, mImplied, 4, false)
	set(0x28, opPLP, mImplied, 4, false)

	// -----------------------------------------------------------------------
	// Arithmetic
	// -----------------------------------------------------------------------
	set(0x69, opADC, mImmediate, 2, false)
	set(0x65, opADC, mZeroPage, 3, false)
	set(0x75, opADC, mZeroPageX, 4, false)
	set(0x6D, opADC, mAbsolute, 4, false)
	set(0x7D, opADC, mAbsoluteX, 4, true)
	set(0x79, opADC, mAbsoluteY, 4, true)
	set(0x61, opADC, mIndirectX, 6, false)
	set(0x71, opADC, mIndirectY, 5, true)

	set(0xE9, opSBC, mImmediate, 2, false)
	set(0xE5, opSBC, mZeroPage, 3, false)
	set(0xF5, opSBC, mZeroPageX, 4, false)
	set(0xED, opSBC, mAbsolute, 4, false)
	set(0xFD, opSBC, mAbsoluteX, 4, true)
	set(0xF9, opSBC, mAbsoluteY, 4, true)
	set(0xE1, opSBC, mIndirectX, 6, false)
	set(0xF1, opSBC, mIndirectY, 5, true)

	// -----------------------------------------------------------------------
	// Logic
	// -----------------------------------------------------------------------
	set(0x29, opAND, mImmediate, 2, false)
	set(0x25, opAND, mZeroPage, 3, false)
	set(0x35, opAND, mZeroPageX, 4, false)
	set(0x2D, opAND, mAbsolute, 4, false)
	set(0x3D, opAND, mAbsoluteX, 4, true)
	set(0x39, opAND, mAbsoluteY, 4, true)
	set(0x21, opAND, mIndirectX, 6, false)
	set(0x31, opAND, mIndirectY, 5, true)

	set(0x09, opORA, mImmediate, 2, false)
	set(0x05, opORA, mZeroPage, 3, false)
	set(0x15, opORA, mZeroPageX, 4, false)
	set(0x0D, opORA, mAbsolute, 4, false)
	set(0x1D, opORA, mAbsoluteX, 4, true)
	set(0x19, opORA, mAbsoluteY, 4, true)
	set(0x01, opORA, mIndirectX, 6, false)
	set(0x11, opORA, mIndirectY, 5, true)

	set(0x49, opEOR, mImmediate, 2, false)
	set(0x45, opEOR, mZeroPage, 3, false)
	set(0x55, opEOR, mZeroPageX, 4, false)
	set(0x4D, opEOR, mAbsolute, 4, false)
	set(0x5D, opEOR, mAbsoluteX, 4, true)
	set(0x59, opEOR, mAbsoluteY, 4, true)
	set(0x41, opEOR, mIndirectX, 6, false)
	set(0x51, opEOR, mIndirectY, 5, true)

	// -----------------------------------------------------------------------
	// Shift / Rotate
	// -----------------------------------------------------------------------
	set(0x0A, opASLA, mAccumulator, 2, false)
	set(0x06, opASL, mZeroPage, 5, false)
	set(0x16, opASL, mZeroPageX, 6, false)
	set(0x0E, opASL, mAbsolute, 6, false)
	set(0x1E, opASL, mAbsoluteX, 7, false)

	set(0x4A, opLSRA, mAccumulator, 2, false)
	set(0x46, opLSR, mZeroPage, 5, false)
	set(0x56, opLSR, mZeroPageX, 6, false)
	set(0x4E, opLSR, mAbsolute, 6, false)
	set(0x5E, opLSR, mAbsoluteX, 7, false)

	set(0x2A, opROLA, mAccumulator, 2, false)
	set(0x26, opROL, mZeroPage, 5, false)
	set(0x36, opROL, mZeroPageX, 6, false)
	set(0x2E, opROL, mAbsolute, 6, false)
	set(0x3E, opROL, mAbsoluteX, 7, false)

	set(0x6A, opRORA, mAccumulator, 2, false)
	set(0x66, opROR, mZeroPage, 5, false)
	set(0x76, opROR, mZeroPageX, 6, false)
	set(0x6E, opROR, mAbsolute, 6, false)
	set(0x7E, opROR, mAbsoluteX, 7, false)

	// -----------------------------------------------------------------------
	// Increment / Decrement
	// -----------------------------------------------------------------------
	set(0xE6, opINC, mZeroPage, 5, false)
	set(0xF6, opINC, mZeroPageX, 6, false)
	set(0xEE, opINC, mAbsolute, 6, false)
	set(0xFE, opINC, mAbsoluteX, 7, false)

	set(0xC6, opDEC, mZeroPage, 5, false)
	set(0xD6, opDEC, mZeroPageX, 6, false)
	set(0xCE, opDEC, mAbsolute, 6, false)
	set(0xDE, opDEC, mAbsoluteX, 7, false)

	set(0xE8, opINX, mImplied, 2, false)
	set(0xC8, opINY, mImplied, 2, false)
	set(0xCA, opDEX, mImplied, 2, false)
	set(0x88, opDEY, mImplied, 2, false)

	// -----------------------------------------------------------------------
	// Compare
	// -----------------------------------------------------------------------
	set(0xC9, opCMP, mImmediate, 2, false)
	set(0xC5, opCMP, mZeroPage, 3, false)
	set(0xD5, opCMP, mZeroPageX, 4, false)
	set(0xCD, opCMP, mAbsolute, 4, false)
	set(0xDD, opCMP, mAbsoluteX, 4, true)
	set(0xD9, opCMP, mAbsoluteY, 4, true)
	set(0xC1, opCMP, mIndirectX, 6, false)
	set(0xD1, opCMP, mIndirectY, 5, true)

	set(0xE0, opCPX, mImmediate, 2, false)
	set(0xE4, opCPX, mZeroPage, 3, false)
	set(0xEC, opCPX, mAbsolute, 4, false)

	set(0xC0, opCPY, mImmediate, 2, false)
	set(0xC4, opCPY, mZeroPage, 3, false)
	set(0xCC, opCPY, mAbsolute, 4, false)

	// -----------------------------------------------------------------------
	// Bit test
	// -----------------------------------------------------------------------
	set(0x24, opBIT, mZeroPage, 3, false)
	set(0x2C, opBIT, mAbsolute, 4, false)

	// -----------------------------------------------------------------------
	// Branches
	// -----------------------------------------------------------------------
	set(0x90, opBCC, mRelative, 2, false)
	set(0xB0, opBCS, mRelative, 2, false)
	set(0xF0, opBEQ, mRelative, 2, false)
	set(0x30, opBMI, mRelative, 2, false)
	set(0xD0, opBNE, mRelative, 2, false)
	set(0x10, opBPL, mRelative, 2, false)
	set(0x50, opBVC, mRelative, 2, false)
	set(0x70, opBVS, mRelative, 2, false)

	// -----------------------------------------------------------------------
	// Jump / Subroutine
	// -----------------------------------------------------------------------
	set(0x4C, opJMP, mAbsolute, 3, false)
	set(0x6C, opJMP, mIndirect, 5, false)
	set(0x20, opJSR, mAbsolute, 6, false)
	set(0x60, opRTS, mImplied, 6, false)
	set(0x40, opRTI, mImplied, 6, false)

	// -----------------------------------------------------------------------
	// Flag operations
	// -----------------------------------------------------------------------
	set(0x18, opCLC, mImplied, 2, false)
	set(0x38, opSEC, mImplied, 2, false)
	set(0xD8, opCLD, mImplied, 2, false)
	set(0xF8, opSED, mImplied, 2, false)
	set(0x58, opCLI, mImplied, 2, false)
	set(0x78, opSEI, mImplied, 2, false)
	set(0xB8, opCLV, mImplied, 2, false)

	// -----------------------------------------------------------------------
	// Misc
	// -----------------------------------------------------------------------
	set(0xEA, opNOP, mImplied, 2, false)
	set(0x00, opBRK, mImplied, 7, false)
}

func set(op byte, fn opHandler, mode addrMode, cycles uint8, pagePenalty bool) {
	opcodeTable[op] = opEntry{fn, mode, cycles, pagePenalty}
}

// ===========================================================================
// Instruction implementations
// ===========================================================================

// ---- Load / Store --------------------------------------------------------

func opLDA(c *CPU, mode addrMode) {
	c.A = c.read(c.resolve(mode))
	c.setZN(c.A)
}

func opLDX(c *CPU, mode addrMode) {
	c.X = c.read(c.resolve(mode))
	c.setZN(c.X)
}

func opLDY(c *CPU, mode addrMode) {
	c.Y = c.read(c.resolve(mode))
	c.setZN(c.Y)
}

func opSTA(c *CPU, mode addrMode) { c.write(c.resolve(mode), c.A) }
func opSTX(c *CPU, mode addrMode) { c.write(c.resolve(mode), c.X) }
func opSTY(c *CPU, mode addrMode) { c.write(c.resolve(mode), c.Y) }

// ---- Register transfers --------------------------------------------------

func opTAX(c *CPU, _ addrMode) { c.X = c.A; c.setZN(c.X) }
func opTAY(c *CPU, _ addrMode) { c.Y = c.A; c.setZN(c.Y) }
func opTXA(c *CPU, _ addrMode) { c.A = c.X; c.setZN(c.A) }
func opTYA(c *CPU, _ addrMode) { c.A = c.Y; c.setZN(c.A) }
func opTSX(c *CPU, _ addrMode) { c.X = c.SP; c.setZN(c.X) }
func opTXS(c *CPU, _ addrMode) { c.SP = c.X } // no flags

// ---- Stack ---------------------------------------------------------------

func opPHA(c *CPU, _ addrMode) { c.push(c.A) }
func opPHP(c *CPU, _ addrMode) { c.push(c.P | FlagB | FlagU) }
func opPLA(c *CPU, _ addrMode) { c.A = c.pull(); c.setZN(c.A) }
func opPLP(c *CPU, _ addrMode) { c.P = c.pull()&^FlagB | FlagU }

// ---- Arithmetic ----------------------------------------------------------

func opADC(c *CPU, mode addrMode) {
	val := c.read(c.resolve(mode))
	a := c.A
	carry := uint16(0)
	if c.getFlag(FlagC) {
		carry = 1
	}

	if c.getFlag(FlagD) {
		// BCD mode (NMOS 6502 behaviour)
		lo := int(a&0x0F) + int(val&0x0F) + int(carry)
		hi := int(a>>4) + int(val>>4)
		if lo > 9 {
			lo -= 10
			hi++
		}
		// Z flag is based on the binary result
		c.setFlag(FlagZ, uint8(uint16(a)+uint16(val)+carry) == 0)
		// N and V are based on the BCD intermediate before high correction
		intermediate := uint8(hi<<4) | uint8(lo&0x0F)
		c.setFlag(FlagN, intermediate&0x80 != 0)
		c.setFlag(FlagV, (a^intermediate)&(val^intermediate)&0x80 != 0)
		if hi > 9 {
			hi -= 10
			c.setFlag(FlagC, true)
		} else {
			c.setFlag(FlagC, false)
		}
		c.A = uint8(hi<<4) | uint8(lo&0x0F)
	} else {
		sum := uint16(a) + uint16(val) + carry
		result := uint8(sum)
		c.setFlag(FlagC, sum > 0xFF)
		c.setFlag(FlagV, (a^result)&(val^result)&0x80 != 0)
		c.A = result
		c.setZN(c.A)
	}
}

func opSBC(c *CPU, mode addrMode) {
	val := c.read(c.resolve(mode))
	a := c.A
	borrow := uint16(0)
	if !c.getFlag(FlagC) {
		borrow = 1
	}

	if c.getFlag(FlagD) {
		// BCD mode (NMOS 6502 behaviour)
		lo := int(a&0x0F) - int(val&0x0F) - int(borrow)
		hi := int(a>>4) - int(val>>4)
		if lo < 0 {
			lo += 10
			hi--
		}
		if hi < 0 {
			hi += 10
			c.setFlag(FlagC, false)
		} else {
			c.setFlag(FlagC, true)
		}
		// N, V, Z based on binary result (NMOS 6502 SBC decimal quirk)
		diff := uint16(a) - uint16(val) - borrow
		c.setFlag(FlagV, (a^uint8(diff))&(a^val)&0x80 != 0)
		c.setZN(uint8(diff))
		c.A = uint8(hi<<4) | uint8(lo&0x0F)
	} else {
		diff := uint16(a) - uint16(val) - borrow
		result := uint8(diff)
		c.setFlag(FlagC, diff < 0x100) // no borrow = carry set
		c.setFlag(FlagV, (a^result)&(a^val)&0x80 != 0)
		c.A = result
		c.setZN(c.A)
	}
}

// ---- Logic ---------------------------------------------------------------

func opAND(c *CPU, mode addrMode) {
	c.A &= c.read(c.resolve(mode))
	c.setZN(c.A)
}

func opORA(c *CPU, mode addrMode) {
	c.A |= c.read(c.resolve(mode))
	c.setZN(c.A)
}

func opEOR(c *CPU, mode addrMode) {
	c.A ^= c.read(c.resolve(mode))
	c.setZN(c.A)
}

// ---- Shift / Rotate (accumulator variants) -------------------------------

func opASLA(c *CPU, _ addrMode) {
	c.setFlag(FlagC, c.A&0x80 != 0)
	c.A <<= 1
	c.setZN(c.A)
}

func opLSRA(c *CPU, _ addrMode) {
	c.setFlag(FlagC, c.A&0x01 != 0)
	c.A >>= 1
	c.setZN(c.A)
}

func opROLA(c *CPU, _ addrMode) {
	carry := uint8(0)
	if c.getFlag(FlagC) {
		carry = 1
	}
	c.setFlag(FlagC, c.A&0x80 != 0)
	c.A = c.A<<1 | carry
	c.setZN(c.A)
}

func opRORA(c *CPU, _ addrMode) {
	carry := uint8(0)
	if c.getFlag(FlagC) {
		carry = 0x80
	}
	c.setFlag(FlagC, c.A&0x01 != 0)
	c.A = c.A>>1 | carry
	c.setZN(c.A)
}

// ---- Shift / Rotate (memory variants) ------------------------------------

func opASL(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr)
	c.setFlag(FlagC, val&0x80 != 0)
	val <<= 1
	c.write(addr, val)
	c.setZN(val)
}

func opLSR(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr)
	c.setFlag(FlagC, val&0x01 != 0)
	val >>= 1
	c.write(addr, val)
	c.setZN(val)
}

func opROL(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr)
	carry := uint8(0)
	if c.getFlag(FlagC) {
		carry = 1
	}
	c.setFlag(FlagC, val&0x80 != 0)
	val = val<<1 | carry
	c.write(addr, val)
	c.setZN(val)
}

func opROR(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr)
	carry := uint8(0)
	if c.getFlag(FlagC) {
		carry = 0x80
	}
	c.setFlag(FlagC, val&0x01 != 0)
	val = val>>1 | carry
	c.write(addr, val)
	c.setZN(val)
}

// ---- Increment / Decrement -----------------------------------------------

func opINC(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr) + 1
	c.write(addr, val)
	c.setZN(val)
}

func opDEC(c *CPU, mode addrMode) {
	addr := c.resolve(mode)
	val := c.read(addr) - 1
	c.write(addr, val)
	c.setZN(val)
}

func opINX(c *CPU, _ addrMode) { c.X++; c.setZN(c.X) }
func opINY(c *CPU, _ addrMode) { c.Y++; c.setZN(c.Y) }
func opDEX(c *CPU, _ addrMode) { c.X--; c.setZN(c.X) }
func opDEY(c *CPU, _ addrMode) { c.Y--; c.setZN(c.Y) }

// ---- Compare -------------------------------------------------------------

func (c *CPU) compare(reg uint8, mode addrMode) {
	val := c.read(c.resolve(mode))
	c.setFlag(FlagC, reg >= val)
	c.setZN(reg - val)
}

func opCMP(c *CPU, mode addrMode) { c.compare(c.A, mode) }
func opCPX(c *CPU, mode addrMode) { c.compare(c.X, mode) }
func opCPY(c *CPU, mode addrMode) { c.compare(c.Y, mode) }

// ---- Bit test ------------------------------------------------------------

func opBIT(c *CPU, mode addrMode) {
	val := c.read(c.resolve(mode))
	c.setFlag(FlagZ, c.A&val == 0)
	c.setFlag(FlagN, val&0x80 != 0)
	c.setFlag(FlagV, val&0x40 != 0)
}

// ---- Branches ------------------------------------------------------------

func opBCC(c *CPU, mode addrMode) { c.branch(mode, !c.getFlag(FlagC)) }
func opBCS(c *CPU, mode addrMode) { c.branch(mode, c.getFlag(FlagC)) }
func opBEQ(c *CPU, mode addrMode) { c.branch(mode, c.getFlag(FlagZ)) }
func opBMI(c *CPU, mode addrMode) { c.branch(mode, c.getFlag(FlagN)) }
func opBNE(c *CPU, mode addrMode) { c.branch(mode, !c.getFlag(FlagZ)) }
func opBPL(c *CPU, mode addrMode) { c.branch(mode, !c.getFlag(FlagN)) }
func opBVC(c *CPU, mode addrMode) { c.branch(mode, !c.getFlag(FlagV)) }
func opBVS(c *CPU, mode addrMode) { c.branch(mode, c.getFlag(FlagV)) }

// ---- Jump / Subroutine ---------------------------------------------------

func opJMP(c *CPU, mode addrMode) {
	c.PC = c.resolve(mode)
}

func opJSR(c *CPU, mode addrMode) {
	target := c.resolve(mode)
	c.push16(c.PC - 1) // push address of last byte of JSR instruction
	c.PC = target
}

func opRTS(c *CPU, _ addrMode) {
	c.PC = c.pull16() + 1
}

func opRTI(c *CPU, _ addrMode) {
	c.P = c.pull()&^FlagB | FlagU
	c.PC = c.pull16()
}

// ---- Flag operations -----------------------------------------------------

func opCLC(c *CPU, _ addrMode) { c.setFlag(FlagC, false) }
func opSEC(c *CPU, _ addrMode) { c.setFlag(FlagC, true) }
func opCLD(c *CPU, _ addrMode) { c.setFlag(FlagD, false) }
func opSED(c *CPU, _ addrMode) { c.setFlag(FlagD, true) }
func opCLI(c *CPU, _ addrMode) { c.setFlag(FlagI, false) }
func opSEI(c *CPU, _ addrMode) { c.setFlag(FlagI, true) }
func opCLV(c *CPU, _ addrMode) { c.setFlag(FlagV, false) }

// ---- Misc ----------------------------------------------------------------

func opNOP(c *CPU, _ addrMode) {}

func opBRK(c *CPU, _ addrMode) {
	c.PC++ // BRK skips the byte after the opcode (padding/signature)
	c.push16(c.PC)
	c.push(c.P | FlagB | FlagU)
	c.setFlag(FlagI, true)
	c.PC = c.read16(0xFFFE)
}
