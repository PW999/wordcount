package audio

import (
	"errors"
	"strings"
	"testing"
)

type fakeBackend struct {
	devs []Device
	err  error
}

func (f fakeBackend) CaptureDevices() ([]Device, error) { return f.devs, f.err }

var testDevices = []Device{
	{Index: 0, Name: "Built-in Audio Analog Stereo"},
	{Index: 1, Name: "USB Podcast Microphone"},
	{Index: 2, Name: "USB Headset"},
}

func TestListDevicesNoMicrophone(t *testing.T) {
	_, err := ListDevices(fakeBackend{})
	if !errors.Is(err, ErrNoDevice) {
		t.Fatalf("err = %v, want ErrNoDevice", err)
	}
	if !strings.Contains(err.Error(), "no microphone found") {
		t.Fatalf("error message %q does not mention missing microphone", err)
	}
}

func TestListDevicesBackendError(t *testing.T) {
	boom := errors.New("boom")
	if _, err := ListDevices(fakeBackend{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want backend error", err)
	}
}

func TestFormatDevices(t *testing.T) {
	want := "0: Built-in Audio Analog Stereo\n1: USB Podcast Microphone\n2: USB Headset\n"
	if got := FormatDevices(testDevices); got != want {
		t.Fatalf("FormatDevices = %q, want %q", got, want)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		wantIndex int
		wantErr   string
	}{
		{"index", "1", 1, ""},
		{"index out of range", "7", 0, "no device with index 7"},
		{"unique substring", "podcast", 1, ""},
		{"substring is case-insensitive", "BUILT-IN", 0, ""},
		{"no match", "bluetooth", 0, `no device matches "bluetooth"`},
		{"ambiguous", "usb", 0, "matches 2 devices"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(testDevices, tc.spec)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Index != tc.wantIndex {
				t.Fatalf("index = %d, want %d", got.Index, tc.wantIndex)
			}
		})
	}
}

func TestResolveErrorListsDevices(t *testing.T) {
	_, err := Resolve(testDevices, "nothing")
	if err == nil || !strings.Contains(err.Error(), "1: USB Podcast Microphone") {
		t.Fatalf("error should include the device list, got %v", err)
	}
}
