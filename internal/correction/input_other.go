//go:build !darwin || !cgo

package correction

func captureInputPlatform() (*Input, error) { return nil, ErrUnsupportedInput }

func captureDestinationPlatform() (*Destination, error) { return nil, ErrUnsupportedInput }
