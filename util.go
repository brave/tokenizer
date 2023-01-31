package main

import (
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
