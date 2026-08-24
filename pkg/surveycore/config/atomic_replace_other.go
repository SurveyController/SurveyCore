//go:build !windows

package config

import "os"

func atomicReplace(source string, target string) error {
	return os.Rename(source, target)
}
