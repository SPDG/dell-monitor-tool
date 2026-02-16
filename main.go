package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	I2C_SLAVE = 0x0703
	DDC_ADDR  = 0x37
	EDID_ADDR = 0x50
)

type FeatureConfig struct {
	VCP    string            `json:"vcp"`
	Values map[string]string `json:"values"`
}

type MonitorConfig struct {
	Model    string                       `json:"model"`
	Match    string                       `json:"match"`
	Features map[string]FeatureConfig     `json:"features"`
	Presets  map[string]map[string]string `json:"presets"`
}

type Device struct {
	Bus    string
	Name   string
	Config *MonitorConfig
}

var verbose bool

func logVerbose(format string, a ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, "DEBUG: "+format+"\n", a...)
	}
}

func getChecksum(data []byte) byte {
	var checksum byte = 0x6E
	for _, b := range data {
		checksum ^= b
	}
	return checksum
}

func parseHex(s string) byte {
	var b byte
	fmt.Sscanf(s, "0x%x", &b)
	if b == 0 {
		fmt.Sscanf(s, "%x", &b)
	}
	return b
}

func parseHex16(s string) uint16 {
	var v uint16
	fmt.Sscanf(s, "0x%x", &v)
	if v == 0 {
		fmt.Sscanf(s, "%x", &v)
	}
	return v
}

func setVCP(bus string, vcp byte, value uint16) error {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), I2C_SLAVE, uintptr(DDC_ADDR)); errno != 0 {
		return errno
	}

	data := []byte{0x51, 0x84, 0x03, vcp, byte(value >> 8), byte(value & 0xFF)}
	data = append(data, getChecksum(data))

	logVerbose("Writing to %s VCP 0x%02x value 0x%04x: %02x", bus, vcp, value, data)

	var lastErr error
	for retry := 0; retry < 3; retry++ {
		_, lastErr = f.Write(data)
		if lastErr == nil {
			return nil
		}
		logVerbose("Retrying write to %s (attempt %d): %v", bus, retry+1, lastErr)
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}

func getVCP(bus string, vcp byte) (uint16, error) {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), I2C_SLAVE, uintptr(DDC_ADDR)); errno != 0 {
		return 0, errno
	}

	req := []byte{0x51, 0x82, 0x01, vcp}
	req = append(req, getChecksum(req))

	if _, err := f.Write(req); err != nil {
		return 0, err
	}

	for retry := 0; retry < 2; retry++ {
		time.Sleep(150 * time.Millisecond)
		reply := make([]byte, 16)
		n, err := f.Read(reply)
		if err == nil && n >= 10 {
			for i := 0; i < n-5; i++ {
				if reply[i] == 0x02 && reply[i+2] == vcp {
					return uint16(reply[i+6])<<8 | uint16(reply[i+7]), nil
				}
			}
			if reply[0] == 0x6e {
				return uint16(reply[8])<<8 | uint16(reply[9]), nil
			}
		}
	}
	return 0, fmt.Errorf("failed read")
}

func getMonitorName(bus string) string {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		return ""
	}
	defer f.Close()

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), I2C_SLAVE, uintptr(EDID_ADDR)); errno != 0 {
		return ""
	}

	edid := make([]byte, 128)
	_, err = f.Read(edid)
	if err != nil {
		return ""
	}

	name := ""
	for j := 54; j < 108; j += 18 {
		if edid[j] == 0 && edid[j+1] == 0 && edid[j+2] == 0 && edid[j+3] == 0xfc {
			name = strings.TrimSpace(string(edid[j+5 : j+18]))
			break
		}
	}

	if name == "" {
		if bytes.Contains(edid, []byte("U4021QW")) {
			name = "Dell U4021QW"
		} else if bytes.Contains(edid, []byte("DELL")) {
			name = "DELL Monitor"
		}
	}
	return name
}

func findConfigFile(customPath string) (string, error) {
	if customPath != "" {
		if _, err := os.Stat(customPath); err == nil {
			return customPath, nil
		}
		return "", fmt.Errorf("config file not found: %s", customPath)
	}

	home, _ := os.UserHomeDir()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	searchPaths := []string{"monitors.json"}
	if xdgConfig != "" {
		searchPaths = append(searchPaths, filepath.Join(xdgConfig, "dell-control", "monitors.json"))
	}
	if home != "" {
		searchPaths = append(searchPaths, filepath.Join(home, ".dell-control", "monitors.json"))
	}

	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no monitors.json found")
}

func loadConfigs(path string) ([]MonitorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var configs []MonitorConfig
	err = json.Unmarshal(data, &configs)
	return configs, err
}

func discoverDevices(configs []MonitorConfig) []Device {
	var devices []Device
	matches, _ := filepath.Glob("/dev/i2c-*")
	for _, bus := range matches {
		name := getMonitorName(bus)
		if name == "" {
			continue
		}

		var matchedConfig *MonitorConfig
		for i := range configs {
			if strings.Contains(strings.ToUpper(name), strings.ToUpper(configs[i].Match)) {
				matchedConfig = &configs[i]
				break
			}
		}
		devices = append(devices, Device{Bus: bus, Name: name, Config: matchedConfig})
	}
	return devices
}

func applyFeature(target *Device, featureName, valueLabel string) error {
	if target.Config == nil {
		return fmt.Errorf("no config for this monitor")
	}
	feat, ok := target.Config.Features[featureName]
	if !ok {
		return fmt.Errorf("feature %s not defined", featureName)
	}
	valStr, ok := feat.Values[strings.ToLower(valueLabel)]
	if !ok {
		return fmt.Errorf("invalid value %s for %s. Options: %v", valueLabel, featureName, feat.Values)
	}
	return setVCP(target.Bus, parseHex(feat.VCP), parseHex16(valStr))
}

func main() {
	configPath := flag.String("config", "", "Path to monitor configurations")
	busPtr := flag.String("bus", os.Getenv("MONITOR_BUS"), "I2C bus (overrides discovery)")
	statusPtr := flag.Bool("status", false, "Show current status")
	inputPtr := flag.String("input", "", "Switch input source")
	pbpPtr := flag.String("pbp", "", "Set PBP mode")
	presetPtr := flag.String("preset", "", "Apply a named preset")
	scanPtr := flag.Bool("scan", false, "Scan VCP codes E0-F2")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.Usage()
		return
	}

	actualConfigPath, err := findConfigFile(*configPath)
	var configs []MonitorConfig
	if err == nil {
		configs, err = loadConfigs(actualConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration from %s: %v\n", actualConfigPath, err)
			fmt.Fprintf(os.Stderr, "Continuing with default settings.\n")
		}
	}

	devices := discoverDevices(configs)

	var target *Device
	if *busPtr != "" {
		for i := range devices {
			if devices[i].Bus == *busPtr {
				target = &devices[i]
				break
			}
		}
		if target == nil {
			target = &Device{Bus: *busPtr, Name: "Manual"}
			if len(configs) > 0 {
				target.Config = &configs[0]
			}
		}
	} else {
		for i := range devices {
			if devices[i].Config != nil {
				target = &devices[i]
				break
			}
		}
		if target == nil && len(devices) > 0 {
			target = &devices[0]
			if len(configs) > 0 {
				target.Config = &configs[0]
			}
		}
	}

	if target == nil {
		fmt.Fprintln(os.Stderr, "No monitor detected.")
		os.Exit(1)
	}

	if *scanPtr {
		fmt.Printf("Scanning VCP codes E0-F2 on %s...\n", target.Bus)
		for code := 0xE0; code <= 0xF2; code++ {
			val, err := getVCP(target.Bus, byte(code))
			if err == nil {
				fmt.Printf("VCP 0x%02X: 0x%04X\n", code, val)
			}
		}
		return
	}

	if *presetPtr != "" {
		if target.Config == nil {
			fmt.Fprintf(os.Stderr, "Error: No config matched for monitor '%s' at %s.\n", target.Name, target.Bus)
			os.Exit(1)
		}
		preset, ok := target.Config.Presets[*presetPtr]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: Preset %s not found.\n", *presetPtr)
			os.Exit(1)
		}

		fmt.Printf("Applying preset: %s on %s\n", *presetPtr, target.Bus)

		// Determine robust application order
		var order []string
		targetPbpMode := preset["pbp_mode"]

		if targetPbpMode == "off" {
			// To Fullscreen: PBP OFF -> Main Input -> USB
			order = []string{"pbp_mode", "input_source", "usb_selection"}
		} else {
			// To PBP: PBP OFF (Reset) -> Main Input -> PBP ON -> Sub Input -> USB
			logVerbose("Resetting PBP before applying preset to ensure clean transition...")
			applyFeature(target, "pbp_mode", "off")
			time.Sleep(2 * time.Second)
			order = []string{"input_source", "pbp_mode", "pbp_sub_input", "usb_selection"}
		}

		for _, featName := range order {
			if valLabel, ok := preset[featName]; ok {
				if err := applyFeature(target, featName, valLabel); err != nil {
					fmt.Fprintf(os.Stderr, "Error applying %s: %v\n", featName, err)
				}
				if featName == "pbp_mode" || featName == "input_source" {
					time.Sleep(2 * time.Second)
				} else {
					time.Sleep(1 * time.Second)
				}
			}
		}
	}

	if *inputPtr != "" {
		applyFeature(target, "input_source", *inputPtr)
	}
	if *pbpPtr != "" {
		applyFeature(target, "pbp_mode", *pbpPtr)
	}

	if *statusPtr || (*inputPtr == "" && *pbpPtr == "" && *presetPtr == "" && !*scanPtr) {
		fmt.Printf("--- Status for %s (%s) ---\n", target.Bus, target.Name)
		if target.Config != nil {
			for name, feat := range target.Config.Features {
				v, _ := getVCP(target.Bus, parseHex(feat.VCP))
				fmt.Printf("%s: 0x%04X\n", name, v)
			}
		}
	}
}
