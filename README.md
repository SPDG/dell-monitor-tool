# Dell Monitor Control Tool

A lightweight Go CLI tool to manage Dell monitor settings (like input sources and Picture-by-Picture) directly via I2C/DDC-CI, with zero external dependencies (no `ddcutil` required).

## Features

- **Direct I2C/DDC-CI Communication**: Fast and low-resource.
- **Dynamic Discovery**: Automatically detects supported monitors on available I2C buses.
- **External Configuration**: Define monitor models, VCP codes, and values in a simple JSON file.
- **Presets**: Apply multiple settings (e.g., input + PBP mode) with a single command.
- **No Hardcoding**: Easily adaptable to other monitor models via `monitors.json`.

## Installation

1. Ensure you have Go installed.
2. Build the binary:
   ```bash
   go build -o dell-control main.go
   ```
3. (Optional) Move to your path:
   ```bash
   sudo mv dell-control /usr/local/bin/
   ```

**Note:** You must have permissions to read/write to `/dev/i2c-*` devices. You might need to add your user to the `i2c` group or run with `sudo`.

## Usage

### Basic Commands
```bash
# Print help
./dell-control

# Show current status (auto-detects monitor)
./dell-control --status

# List all discovered monitors
./dell-control --detect

# Switch input source
./dell-control --input hdmi1
```

### Using Presets
Presets are defined in `monitors.json` and allow for complex state changes:
```bash
# Switch to a pre-defined setup
./dell-control --preset divided
```

## Configuration

The tool searches for `monitors.json` in the following locations:
1. Current working directory.
2. `$XDG_CONFIG_HOME/dell-control/monitors.json` (usually `~/.config/dell-control/`).
3. `$HOME/.dell-control/monitors.json`.

### Example `monitors.json`
```json
[
  {
    "model": "Dell U4021QW",
    "match": "U4021QW",
    "features": {
      "input_source": {
        "vcp": "0x60",
        "values": { "dp": "0x0f", "hdmi1": "0x11" }
      }
    },
    "presets": {
      "work": { "input_source": "dp" }
    }
  }
]
```

## Environment Variables
- `MONITOR_BUS`: Set this to a specific bus (e.g., `/dev/i2c-7`) to bypass discovery.

## CI/CD
- **CI**: Every push to `master` triggers a build and test suite.
- **Releases**: Pushing a tag (e.g., `v1.0.0`) automatically creates a GitHub release with pre-built binaries for Linux (amd64 and arm64).
