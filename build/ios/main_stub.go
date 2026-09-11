//go:build !ios

package main

// The iOS support files live in this directory so Wails can assemble the
// Xcode project. Keep the generated directory buildable on regular hosts when
// repository-wide Go checks walk every package; the real application entry
// point is supplied by the iOS overlay during an iOS build.
func main() {}
