package main

import (
	"testing"
)

func TestForwarderStartStop(t *testing.T) {
	c := &config{
		kafkaConfig: createKafkaConf(t),
	}
	for _, newForwarder := range ourForwarders {
		f := newForwarder()
		f.setConfig(c)
		f.start()
		f.stop()
	}
}
