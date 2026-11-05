//go:build !embedffmpeg || !windows

package main

func extractEmbeddedFFmpeg() (string, error) { return "", nil }
