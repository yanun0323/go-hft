package enum

type Platform uint8

const (
	_platform_beg Platform = iota
	PlatformBTCC
	PlatformBinance
	_platform_end
)

func (p Platform) IsAvailable() bool {
	return p > _platform_beg && p < _platform_end
}
