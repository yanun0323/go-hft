package enum

type Platform uint8

const (
	platform_beg Platform = iota
	PlatformBTCC
	PlatformBinance
	platform_end
)

func (p Platform) IsAvailable() bool {
	return p > platform_beg && p < platform_end
}
