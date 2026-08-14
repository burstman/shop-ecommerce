package main

import "embed"

// This line tells Go to bundle the public folder into the binary
//
//go:embed public/*
var PublicFS embed.FS
