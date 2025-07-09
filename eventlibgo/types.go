package eventlib

// EventType represents the type of event
type EventType int

const (
	EventTypeData       EventType = 0
	EventTypeConnect    EventType = 1
	EventTypeDisconnect EventType = 2
	EventTypeError      EventType = 3
)

func (et EventType) String() string {
	switch et {
	case EventTypeData:
		return "DATA"
	case EventTypeConnect:
		return "CONNECT"
	case EventTypeDisconnect:
		return "DISCONNECT"
	case EventTypeError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Event represents an event in the system
type Event struct {
	Type   EventType
	Source string
	Data   []byte
}

// Handler function types
type (
	EventHandler       func(event Event)
	FilterHandler      func(event Event) bool
	StateChangeHandler func(oldState, newState string)
)
