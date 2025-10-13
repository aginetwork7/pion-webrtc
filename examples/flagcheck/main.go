package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/pion/webrtc/v4/pkg/flagcheck"
)

func main() {
	ok, err := flagcheck.CheckSupportAcceleration()
	if err != nil && errors.Is(err, os.ErrNotExist) {
		fmt.Println("hardware acceleration is available")
		return
	}
	if ok {
		fmt.Println("hardware acceleration is available")
	} else {
		fmt.Println("hardware acceleration not available")
	}
}
