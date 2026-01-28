package adapter

import "testing"

func BenchmarkDepthByteSize(b *testing.B) {
	d := Depth{}
	for b.Loop() {
		b := d.SizeInByte()
		_ = b
	}
}

func BenchmarkDepthEncode(b *testing.B) {
	d := Depth{}
	buf := make([]byte, d.SizeInByte())
	for b.Loop() {
		buf := d.Encode(buf)
		_ = buf
	}
}
