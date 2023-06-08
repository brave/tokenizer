package main

import (
	"syscall"

	"github.com/linkedin/goavro/v2"
)

func avroEncode(codec *goavro.Codec, blob []byte) ([]byte, error) {
	native, _, err := codec.NativeFromTextual(blob)
	if err != nil {
		return nil, err
	}
	binary, err := codec.BinaryFromNative(nil, native)
	return binary, err
}

// maxSoftFdLimit raises the file descriptor soft limit to the hard limit.
func maxSoftFdLimit() error {
	var rLimit = new(syscall.Rlimit)
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, rLimit); err != nil {
		return err
	}
	l.Printf("Original fd limit: cur=%d, max=%d", rLimit.Cur, rLimit.Max)

	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, rLimit); err != nil {
		return err
	}
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, rLimit); err != nil {
		return err
	}
	l.Printf("Modified fd limit: cur=%d, max=%d.", rLimit.Cur, rLimit.Max)

	return nil
}
