package utils

import (
	"testing"
)

func TestIsAudioFile(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		expected    bool
	}{
		// Test by extension
		{"mp3 extension", "audio.mp3", "", true},
		{"wav extension", "recording.wav", "", true},
		{"ogg extension", "sound.ogg", "", true},
		{"m4a extension", "voice.m4a", "", true},
		{"flac extension", "music.flac", "", true},
		{"aac extension", "track.aac", "", true},
		{"wma extension", "audio.wma", "", true},
		{"uppercase extension", "AUDIO.MP3", "", true},
		{"mixed case extension", "Audio.Mp3", "", true},

		// Test by content type
		{"audio/mp4 content type", "file.bin", "audio/mp4", true},
		{"audio/mpeg content type", "file.bin", "audio/mpeg", true},
		{"audio/ogg content type", "file.bin", "audio/ogg", true},
		{"application/ogg content type", "file.bin", "application/ogg", true},
		{"application/x-ogg content type", "file.bin", "application/x-ogg", true},
		{"uppercase content type", "file.bin", "AUDIO/MPEG", true},

		// Test non-audio files
		{"txt file", "document.txt", "text/plain", false},
		{"pdf file", "doc.pdf", "application/pdf", false},
		{"jpg image", "photo.jpg", "image/jpeg", false},
		{"mp4 video", "video.mp4", "video/mp4", false},
		{"unknown extension", "file.xyz", "", false},
		{"empty filename", "", "", false},

		// Edge cases
		{"dot in middle", "my.audio.file.mp3", "", true},
		{"multiple dots", "file..mp3", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAudioFile(tt.filename, tt.contentType)
			if got != tt.expected {
				t.Errorf("IsAudioFile(%q, %q) = %v, want %v", tt.filename, tt.contentType, got, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected string
	}{
		{"normal filename", "document.pdf", "document.pdf"},
		{"filename with path", "/home/user/file.txt", "file.txt"},
		{"windows path", "C:\\Users\\file.doc", "C:_Users_file.doc"},
		{"directory traversal", "../etc/passwd", "passwd"},
		{"double dots", "..hidden", "hidden"},
		{"mixed separators", "path/to\\file", "to_file"},
		{"only dots", "...", "."},
		{"empty string", "", "."},
		{"special characters kept", "my-file_v1.0.txt", "my-file_v1.0.txt"},
		{"unicode characters", "文件.txt", "文件.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.filename)
			if got != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.filename, got, tt.expected)
			}
		})
	}
}
