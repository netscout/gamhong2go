package memory

import "testing"

func TestRAM_ZeroInit(t *testing.T) {
	r := NewRAM()
	for i := 0; i < 512; i++ {
		if r.Data[i] != 0 {
			t.Errorf("addr $%04X: want $00, got $%02X", i, r.Data[i])
		}
	}
}

func TestRAM_ReadWrite(t *testing.T) {
	r := NewRAM()
	r.Write(0x0200, 0xAB)
	if got := r.Read(0x0200); got != 0xAB {
		t.Errorf("Read after Write: want $AB, got $%02X", got)
	}
}
