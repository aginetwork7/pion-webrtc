package flagcheck

import (
	"fmt"
	"testing"
)

func TestFlagCheck(t *testing.T) {
	ok, err := CheckSupportAcceleration()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if ok {
		fmt.Println("hardware acceleration is available")
	} else {
		fmt.Println("hardware acceleration not available")
	}
}
