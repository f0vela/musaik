//go:build !darwin

package main

var np = struct{ available bool }{}

func (a *app) setupNowPlaying() bool { return false }
func (a *app) updateNowPlaying()     {}
func (a *app) clearNowPlaying()      {}
