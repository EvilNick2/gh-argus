// Package mouse is the mouse event the app hands each view, already turned
// into the view's own coordinates, with double clicks recognised.
package mouse

type Kind int

const (
	Click Kind = iota
	DoubleClick
	WheelUp
	WheelDown
)

// Event is a mouse action at X, Y relative to the receiving view's top left.
type Event struct {
	X, Y int
	Kind Kind
}

// Shift moves the event's origin, for handing it to a part of a view.
func (e Event) Shift(dx, dy int) Event {
	e.X, e.Y = e.X-dx, e.Y-dy
	return e
}

// Wheel is -1 for wheel up, 1 for wheel down, otherwise 0.
func (e Event) Wheel() int {
	switch e.Kind {
	case WheelUp:
		return -1
	case WheelDown:
		return 1
	}
	return 0
}

// Clicked reports a single or double click.
func (e Event) Clicked() bool {
	return e.Kind == Click || e.Kind == DoubleClick
}
