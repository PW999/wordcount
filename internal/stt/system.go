package stt

import whispercpp "github.com/ggerganov/whisper.cpp/bindings/go"

// SystemInfo reports the CPU features whisper.cpp was built with.
func SystemInfo() string {
	return whispercpp.Whisper_print_system_info()
}
