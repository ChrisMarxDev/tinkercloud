package webui

import (
	"encoding/base64"
)

const (
	qrVersion       = 10
	qrSize          = qrVersion*4 + 17
	qrDataCodewords = 274
	qrECCCodewords  = 18
	qrMaxBytes      = 271
)

// qrCodeData returns a dependency-free, byte-mode QR matrix for a stable app URL.
// Version 10-L covers every normally valid HTTPS DNS URL while keeping the
// native UI self-contained. Unsupported input renders no control rather than a
// broken or remotely sourced image.
func qrCodeData(value string) string {
	data := []byte(value)
	if len(data) == 0 || len(data) > qrMaxBytes {
		return ""
	}
	modules := qrModules(data)
	packed := make([]byte, (qrSize*qrSize+7)/8)
	for y, row := range modules {
		for x, dark := range row {
			if dark {
				index := y*qrSize + x
				packed[index>>3] |= 1 << uint(7-(index&7))
			}
		}
	}
	return base64.StdEncoding.EncodeToString(packed)
}

func qrModules(data []byte) [][]bool {
	codewords := qrCodewords(data)
	modules := make([][]bool, qrSize)
	function := make([][]bool, qrSize)
	for i := range modules {
		modules[i] = make([]bool, qrSize)
		function[i] = make([]bool, qrSize)
	}

	set := func(x, y int, dark bool) {
		modules[y][x] = dark
		function[y][x] = true
	}
	drawFinder := func(cx, cy int) {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := cx+dx, cy+dy
				if x < 0 || x >= qrSize || y < 0 || y >= qrSize {
					continue
				}
				distance := max(abs(dx), abs(dy))
				set(x, y, distance != 2 && distance != 4)
			}
		}
	}
	drawAlignment := func(cx, cy int) {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				set(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
			}
		}
	}

	for i := 0; i < qrSize; i++ {
		set(6, i, i%2 == 0)
		set(i, 6, i%2 == 0)
	}
	drawFinder(3, 3)
	drawFinder(qrSize-4, 3)
	drawFinder(3, qrSize-4)
	for _, y := range []int{6, 28, 50} {
		for _, x := range []int{6, 28, 50} {
			if (x == 6 && y == 6) || (x == 6 && y == 50) || (x == 50 && y == 6) {
				continue
			}
			drawAlignment(x, y)
		}
	}
	drawVersionBits(set)
	drawFormatBits(set, 0)

	bit := 0
	for right := qrSize - 1; right >= 1; right -= 2 {
		if right == 6 {
			right--
		}
		upward := (right+1)&2 == 0
		for vertical := 0; vertical < qrSize; vertical++ {
			y := vertical
			if upward {
				y = qrSize - 1 - vertical
			}
			for offset := 0; offset < 2; offset++ {
				x := right - offset
				if function[y][x] {
					continue
				}
				if bit < len(codewords)*8 {
					modules[y][x] = codewords[bit>>3]&(1<<uint(7-(bit&7))) != 0
					bit++
				}
			}
		}
	}

	// Mask 0 is a standards-defined mask. A fixed mask keeps this small and
	// deterministic; the surrounding quiet zone preserves scanner contrast.
	for y := 0; y < qrSize; y++ {
		for x := 0; x < qrSize; x++ {
			if !function[y][x] && (x+y)%2 == 0 {
				modules[y][x] = !modules[y][x]
			}
		}
	}
	return modules
}

func qrCodewords(data []byte) []byte {
	bits := make([]bool, 0, qrDataCodewords*8)
	appendBits := func(value, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, value&(1<<uint(i)) != 0)
		}
	}
	appendBits(0x4, 4)
	appendBits(len(data), 16)
	for _, value := range data {
		appendBits(int(value), 8)
	}
	for i := 0; i < 4 && len(bits) < qrDataCodewords*8; i++ {
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	raw := make([]byte, len(bits)/8)
	for i, value := range bits {
		if value {
			raw[i>>3] |= 1 << uint(7-(i&7))
		}
	}
	for pad := byte(0xEC); len(raw) < qrDataCodewords; pad ^= 0xEC ^ 0x11 {
		raw = append(raw, pad)
	}

	blockLengths := [...]int{68, 68, 69, 69}
	blocks := make([][]byte, len(blockLengths))
	ecc := make([][]byte, len(blockLengths))
	offset := 0
	divisor := qrReedSolomonDivisor(qrECCCodewords)
	for i, length := range blockLengths {
		blocks[i] = append([]byte(nil), raw[offset:offset+length]...)
		ecc[i] = qrReedSolomonRemainder(blocks[i], divisor)
		offset += length
	}
	result := make([]byte, 0, 346)
	for i := 0; i < 69; i++ {
		for _, block := range blocks {
			if i < len(block) {
				result = append(result, block[i])
			}
		}
	}
	for i := 0; i < qrECCCodewords; i++ {
		for _, block := range ecc {
			result = append(result, block[i])
		}
	}
	return result
}

func drawFormatBits(set func(int, int, bool), mask int) {
	data := 1<<3 | mask // Error-correction level L has format bits 01.
	remainder := data
	for i := 0; i < 10; i++ {
		remainder = remainder<<1 ^ (remainder>>9)*0x537
	}
	bits := (data<<10 | remainder) ^ 0x5412
	get := func(i int) bool { return bits&(1<<uint(i)) != 0 }
	for i := 0; i <= 5; i++ {
		set(8, i, get(i))
	}
	set(8, 7, get(6))
	set(8, 8, get(7))
	set(7, 8, get(8))
	for i := 9; i < 15; i++ {
		set(14-i, 8, get(i))
	}
	for i := 0; i < 8; i++ {
		set(qrSize-1-i, 8, get(i))
	}
	for i := 8; i < 15; i++ {
		set(8, qrSize-15+i, get(i))
	}
	set(8, qrSize-8, true)
}

func drawVersionBits(set func(int, int, bool)) {
	remainder := qrVersion
	for i := 0; i < 12; i++ {
		remainder = remainder<<1 ^ (remainder>>11)*0x1F25
	}
	bits := qrVersion<<12 | remainder
	for i := 0; i < 18; i++ {
		dark := bits&(1<<uint(i)) != 0
		a, b := qrSize-11+i%3, i/3
		set(a, b, dark)
		set(b, a, dark)
	}
}

func qrReedSolomonDivisor(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			result[j] = qrGFMultiply(result[j], root)
			if j+1 < degree {
				result[j] ^= result[j+1]
			}
		}
		root = qrGFMultiply(root, 0x02)
	}
	return result
}

func qrReedSolomonRemainder(data, divisor []byte) []byte {
	result := make([]byte, len(divisor))
	for _, value := range data {
		factor := value ^ result[0]
		copy(result, result[1:])
		result[len(result)-1] = 0
		for i := range result {
			result[i] ^= qrGFMultiply(divisor[i], factor)
		}
	}
	return result
}

func qrGFMultiply(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = z<<1 ^ (z>>7)*0x11D
		z ^= ((int(y) >> uint(i)) & 1) * int(x)
	}
	return byte(z)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
