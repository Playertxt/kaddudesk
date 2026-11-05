//go:build !embedffmpeg

package main

func extractEmbeddedFFmpeg() (string, error) { return "", nil }
