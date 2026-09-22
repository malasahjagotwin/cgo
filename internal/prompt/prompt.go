// Package prompt membangun prompt shell dengan teks bergradasi warna.
//
// Untuk menyesuaikan tampilan, cukup ubah DefaultTheme di bawah:
// warna awal (Start) -> warna akhir (End) diterapkan per-karakter,
// sementara kurung "[" "]" dan "@" memakai warna Bracket.
package prompt

import (
	"fmt"
	"strings"
)

// RGB adalah satu warna 24-bit (truecolor).
type RGB struct{ R, G, B int }

// Theme mendefinisikan skema warna prompt. Edit di sini untuk kustomisasi.
type Theme struct {
	Start   RGB // warna awal gradasi teks
	End     RGB // warna akhir gradasi teks
	Bracket RGB // warna untuk "[", "]", dan "@"
}

// DefaultTheme: gradasi merah -> abu-abu -> putih, kurung & @ putih.
var DefaultTheme = Theme{
	Start:   RGB{220, 40, 40},   // merah
	End:     RGB{255, 255, 255}, // putih (abu-abu terlewati di tengah gradasi)
	Bracket: RGB{255, 255, 255}, // putih
}

const reset = "\x1b[0m"

func fg(c RGB) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
}

func lerp(a, b, t float64) int {
	return int(a + (b-a)*t)
}

// Gradient mewarnai teks dengan gradasi Start -> End (hanya teksnya).
func (th Theme) Gradient(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range runes {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		c := RGB{
			R: lerp(float64(th.Start.R), float64(th.End.R), t),
			G: lerp(float64(th.Start.G), float64(th.End.G), t),
			B: lerp(float64(th.Start.B), float64(th.End.B), t),
		}
		b.WriteString(fg(c))
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}

// wrap membungkus teks polos dengan warna Bracket.
func (th Theme) bracket(s string) string {
	return fg(th.Bracket) + s + reset
}

// Build menyusun prompt [username@hostname] dengan kurung/@ berwarna
// Bracket dan username/hostname bergradasi.
func (th Theme) Build(username, hostname string) string {
	return th.bracket("[") +
		th.Gradient(username) +
		th.bracket("@") +
		th.Gradient(hostname) +
		th.bracket("]") + " "
}
