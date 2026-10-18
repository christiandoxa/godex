//go:build !windows

package main

import "os"

func secureFixtureFile(path string) error { return os.Chmod(path, 0600) }

func fixtureSecurityMetadata(string) string { return "" }
