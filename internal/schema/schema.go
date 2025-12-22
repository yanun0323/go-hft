package schema

// SchemaVersion is the current event schema version.
const SchemaVersion uint16 = 1

// EventType defines the category of an event stored in the WAL.
type EventType uint16

const (
	EventUnknown EventType = iota
	EventMarketData
	EventOrderIntent
	EventOrderAck
	EventFill
	EventRiskDecision
	EventStrategyDecision
)

// EventHeader is the common metadata attached to every event.
type EventHeader struct {
	Type    EventType
	Version uint16
	Source  uint16
	Flags   uint16
	Seq     uint64
	TsEvent int64
	TsRecv  int64
	TraceID uint64
}

// NewHeader builds a header with the current schema version.
func NewHeader(eventType EventType, source uint16, seq uint64, tsEvent, tsRecv int64) EventHeader {
	return EventHeader{
		Type:    eventType,
		Version: SchemaVersion,
		Source:  source,
		Seq:     seq,
		TsEvent: tsEvent,
		TsRecv:  tsRecv,
	}
}
