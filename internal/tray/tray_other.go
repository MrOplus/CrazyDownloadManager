//go:build !windows && !darwin

package tray

// Start reports false: there is no tray on this system yet.
func Start(Menu) bool { return false }

func SetStatus(string) {}

func Stop() {}
