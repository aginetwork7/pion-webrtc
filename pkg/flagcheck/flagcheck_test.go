package flagcheck

import (
	"fmt"
	"os"
	"testing"

	"github.com/pkg/errors"
)

func TestFlagCheck(t *testing.T) {
	ok, err := CheckSupportAcceleration()
	if err != nil && errors.Is(err, os.ErrNotExist) {
		fmt.Println("hardware acceleration may be available")
		return
	}
	if ok {
		fmt.Println("hardware acceleration is available")
	} else {
		fmt.Println("hardware acceleration not available")
	}
}
