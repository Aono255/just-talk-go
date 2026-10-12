//go:build !darwin

package main

func runPlatformMain(run func()) { run() }
