package enum

type Topic uint8

const (
	_topic_beg Topic = iota
	TopicDepth
	TopicOrder
	_topic_end
)

func (t Topic) IsAvailable() bool {
	return t > _topic_beg && t < _topic_end
}
