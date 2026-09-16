package main

import (
	"errors"
	"io"
	"net/http"
)

// fetchRunnerBufferedFrame is the playback variant of fetchRunnerFrame. Point
// reads (notably the terminal snapshot captured at finish) keep using the
// runner's newest frame, while live viewers explicitly drain its short
// emulator-time buffer one image at a time.
func fetchRunnerBufferedFrame(addrs []string) ([]byte, error) {
	client := frameClient
	for _, addr := range addrs {
		up, err := client.Get("http://" + addr + "/frame.png?buffered=1")
		if err != nil {
			continue
		}
		if up.StatusCode != http.StatusOK {
			up.Body.Close()
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(up.Body, maxRunnerFrameBytes+1))
		up.Body.Close()
		if rerr != nil || len(data) > maxRunnerFrameBytes {
			continue
		}
		return data, nil
	}
	return nil, errors.New("runner frame unavailable")
}
