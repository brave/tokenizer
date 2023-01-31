package main

import (
	"encoding/json"
	"testing"
)

func TestAvroEncode(t *testing.T) {
	// Encode our blob.
	orig := []byte(`{
		"wallet_id": "655f6eab-a6c0-4712-9d57-d3ef09a423e9",
		"service": "ADS",
		"signal": "ANON_IP_ADDRS",
		"score": 0,
		"justification": "{\"keyid\":\"fd1ecf36-f5f5-4186-888d-d3ef09a423e9\",\"addrs\":[\"1.1.1.1\",\"2.2.2.2\"]}",
		"created_at":"2021-12-16T12:00:00.00+00:20"
	}`)
	encoded, err := avroEncode(ourCodec, orig)
	if err != nil {
		t.Fatalf("Failed to Avro-encode data: %v", err)
	}

	// Now try to decode it.
	native, _, err := ourCodec.NativeFromBinary(encoded)
	if err != nil {
		t.Fatalf("Failed to get native from binary: %v", err)
	}
	decoded, err := ourCodec.TextualFromNative(nil, native)
	if err != nil {
		t.Fatalf("Failed to get textual from native: %v", err)
	}
	origStruct := kafkaMessage{}
	decodedStruct := kafkaMessage{}
	if err := json.Unmarshal(orig, &origStruct); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	if err := json.Unmarshal(decoded, &decodedStruct); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Did we end up with the same struct?
	if origStruct != decodedStruct {
		t.Fatalf("Expected\n%v\nbut got\n%v", origStruct, decodedStruct)
	}
}
