package devices

import (
	"testing"
)

func TestParseHLSMICSV(t *testing.T) {
	t.Run("parses full 8-device output", func(t *testing.T) {
		csv := `index, name, bus_id, driver_version, uuid, module_id, memory.total [MiB], memory.free [MiB], memory.used [MiB]
0, HL-325L, 0000:3b:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAN071-12-03-01, 1, 131072 MiB, 130400 MiB, 672 MiB
1, HL-325L, 0000:4c:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAN071-05-08-05, 3, 131072 MiB, 130400 MiB, 672 MiB
2, HL-325L, 0000:5d:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAS681-15-03-02, 2, 131072 MiB, 130400 MiB, 672 MiB
3, HL-325L, 0000:9b:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAN071-04-07-03, 6, 131072 MiB, 130400 MiB, 672 MiB
4, HL-325L, 0000:19:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAN071-08-07-01, 0, 131072 MiB, 130400 MiB, 672 MiB
5, HL-325L, 0000:bb:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAH066-20-03-04, 7, 131072 MiB, 130400 MiB, 672 MiB
6, HL-325L, 0000:cb:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAN075-04-06-07, 5, 131072 MiB, 130400 MiB, 672 MiB
7, HL-325L, 0000:db:00.0, 1.24.1-b336d5e, 01P4-HL3090A0-18-UAH079-13-03-03, 4, 131072 MiB, 130400 MiB, 672 MiB
`
		devices, err := parseHLSMICSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(devices) != 8 {
			t.Fatalf("expected 8 devices, got %d", len(devices))
		}

		d := devices[0]
		if d.Index != 0 {
			t.Errorf("expected index 0, got %d", d.Index)
		}
		if d.Name != productHL325L {
			t.Errorf("expected name HL-325L, got %s", d.Name)
		}
		if d.BusID != "0000:3b:00.0" {
			t.Errorf("expected bus ID 0000:3b:00.0, got %s", d.BusID)
		}
		if d.DriverVersion != "1.24.1-b336d5e" {
			t.Errorf("expected driver 1.24.1-b336d5e, got %s", d.DriverVersion)
		}
		if d.UUID != "01P4-HL3090A0-18-UAN071-12-03-01" {
			t.Errorf("expected UUID 01P4-HL3090A0-18-UAN071-12-03-01, got %s", d.UUID)
		}
		if d.ModuleID != 1 {
			t.Errorf("expected module ID 1, got %d", d.ModuleID)
		}
		if d.MemoryTotal != 131072 {
			t.Errorf("expected memory total 131072, got %d", d.MemoryTotal)
		}
		if d.MemoryFree != 130400 {
			t.Errorf("expected memory free 130400, got %d", d.MemoryFree)
		}
		if d.MemoryUsed != 672 {
			t.Errorf("expected memory used 672, got %d", d.MemoryUsed)
		}

		last := devices[7]
		if last.Index != 7 {
			t.Errorf("expected last device index 7, got %d", last.Index)
		}
		if last.BusID != "0000:db:00.0" {
			t.Errorf("expected last bus ID 0000:db:00.0, got %s", last.BusID)
		}
	})

	t.Run("parses single device", func(t *testing.T) {
		csv := `index, name, bus_id, driver_version, uuid, module_id, memory.total [MiB], memory.free [MiB], memory.used [MiB]
0, HL-225, 0000:3b:00.0, 1.20.0-abc123, some-uuid-here, 0, 98304 MiB, 97000 MiB, 1304 MiB
`
		devices, err := parseHLSMICSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(devices) != 1 {
			t.Fatalf("expected 1 device, got %d", len(devices))
		}
		if devices[0].Name != productHL225 {
			t.Errorf("expected name HL-225, got %s", devices[0].Name)
		}
		if devices[0].MemoryTotal != 98304 {
			t.Errorf("expected memory total 98304, got %d", devices[0].MemoryTotal)
		}
	})

	t.Run("returns error on empty output", func(t *testing.T) {
		_, err := parseHLSMICSV("")
		if err == nil {
			t.Fatal("expected error for empty output")
		}
	})

	t.Run("returns error on header only", func(t *testing.T) {
		csv := `index, name, bus_id, driver_version, uuid, module_id, memory.total [MiB], memory.free [MiB], memory.used [MiB]
`
		_, err := parseHLSMICSV(csv)
		if err == nil {
			t.Fatal("expected error for header-only output")
		}
	})

	t.Run("skips malformed lines", func(t *testing.T) {
		csv := `index, name, bus_id, driver_version, uuid, module_id, memory.total [MiB], memory.free [MiB], memory.used [MiB]
not-a-number, HL-325L, 0000:3b:00.0, 1.24.1, uuid1, 0, 131072 MiB, 130400 MiB, 672 MiB
0, HL-325L, 0000:4c:00.0, 1.24.1, uuid2, 1, 131072 MiB, 130400 MiB, 672 MiB
too, few, fields
1, HL-325L, 0000:5d:00.0, 1.24.1, uuid3, 2, 131072 MiB, 130400 MiB, 672 MiB
`
		devices, err := parseHLSMICSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(devices) != 2 {
			t.Fatalf("expected 2 valid devices, got %d", len(devices))
		}
		if devices[0].Index != 0 {
			t.Errorf("expected first valid device index 0, got %d", devices[0].Index)
		}
		if devices[1].Index != 1 {
			t.Errorf("expected second valid device index 1, got %d", devices[1].Index)
		}
	})
}

func TestParseMiBValue(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"131072 MiB", 131072},
		{"130400 MiB", 130400},
		{"672 MiB", 672},
		{"239 W", 239},
		{"0 MiB", 0},
		{"131072", 131072},
		{" 131072 MiB ", 131072},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMiBValue(tt.input)
			if got != tt.expected {
				t.Errorf("parseMiBValue(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestProductToArch(t *testing.T) {
	tests := []struct {
		product  string
		expected string
	}{
		{productHL325L, archGaudi3},
		{productHL325, archGaudi3},
		{productHL225, archGaudi2},
		{productHL225B, archGaudi2},
	}

	for _, tt := range tests {
		t.Run(tt.product, func(t *testing.T) {
			got, ok := productToArch[tt.product]
			if !ok {
				t.Fatalf("product %q not found in productToArch", tt.product)
			}
			if got != tt.expected {
				t.Errorf("productToArch[%q] = %q, want %q", tt.product, got, tt.expected)
			}
		})
	}

	t.Run("unknown product returns false", func(t *testing.T) {
		_, ok := productToArch["UNKNOWN-DEVICE"]
		if ok {
			t.Error("expected unknown product to not be in map")
		}
	})
}
