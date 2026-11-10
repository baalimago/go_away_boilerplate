package cmd

// Hider is a Command that stays callable and appears in no listing.
type Hider interface {
	Hidden() bool
}

// IsHidden reports whether c asks to stay out of help and completion.
func IsHidden(c Command) bool {
	hider, ok := c.(Hider)
	return ok && hider.Hidden()
}
