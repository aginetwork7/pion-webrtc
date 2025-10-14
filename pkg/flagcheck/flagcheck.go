package flagcheck

import (
	"bufio"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// Check if the CPU supports hardware-accelerated instruction sets (such as Intel AES-NI or ARMv8 AES)
func checkCPUFlags() (bool, error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		if (strings.HasPrefix(line, "flags") || strings.HasPrefix(line, "features")) && strings.Contains(line, "aes") {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// Check if the kernel has loaded the hardware acceleration algorithm module
func checkKernelCrypto() (bool, error) {
	file, err := os.Open("/proc/crypto")
	if err != nil {
		return false, err
	}
	defer file.Close()

	var (
		name   string
		driver string
		typ    string
	)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if strings.Contains(strings.ToLower(name), "aes") && (typ == "cipher" || typ == "skcipher") {
				if !strings.Contains(strings.ToLower(driver), "generic") &&
					!strings.Contains(strings.ToLower(driver), "null") {
					return true, nil
				}
			}
			name, driver, typ = "", "", ""
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			name = val
		case "driver":
			driver = val
		case "type":
			typ = val
		}
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func CheckSupportAcceleration() (bool, error) {
	hasCPU, err := checkCPUFlags()
	if err != nil {
		return false, errors.Wrap(err, "failed to check CPU flags")
	}
	if !hasCPU {
		return false, nil
	}

	hasHW, err := checkKernelCrypto()
	if err != nil {
		return false, errors.Wrap(err, "failed to check kernel crypto")
	}
	if !hasHW {
		return false, nil
	}

	return true, nil
}
