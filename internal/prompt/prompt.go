package prompt

import (
	"fmt"
	"strings"
)

type RGB struct{ R, G, B int }

type Theme struct {
	Start   RGB
	End     RGB
	Bracket RGB
}

var DefaultTheme = Theme{
	Start:   RGB{220, 40, 40},
	End:     RGB{255, 255, 255},
	Bracket: RGB{255, 255, 255},
}

const reset = "\x1b[0m"

func fg(c RGB) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
}

func lerp(a, b, t float64) int {
	return int(a + (b-a)*t)
}

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

func (th Theme) GradientUnderline(s string) string {
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
		b.WriteString(fmt.Sprintf("\x1b[4;38;2;%d;%d;%dm", c.R, c.G, c.B))
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}

func (th Theme) bracket(s string) string {
	return fg(th.Bracket) + s + reset
}

func (th Theme) Build(username, hostname string) string {
	return th.bracket("[") +
		th.Gradient(username) +
		th.bracket("@") +
		th.Gradient(hostname) +
		th.bracket("]") + " "
}
