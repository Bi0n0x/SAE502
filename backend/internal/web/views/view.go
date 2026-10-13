// Package views holds the server-rendered HTML, written as templ components.
// Run `go tool templ generate` after editing a .templ file.
package views

//go:generate go tool templ generate

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/casino-rt/backend/internal/web"
)

type User struct {
	Name    string
	Balance int64

	// Set after a round: the chip shows Previous, then rolls to Balance once
	// the animation has had time to finish.
	Previous int64
	Delay    float64
}

type Page struct {
	Title  string
	Active string
	User   *User

	// Game is the key of the game the page belongs to. It selects the colour
	// theme of the whole page.
	Game string
}

func (p Page) forGame(key string) Page {
	p.Game = key
	return p
}

// initials is what the avatar shows: the first letter of the first two parts
// of the name, or its first two letters when it has a single part.
func initials(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == '.' })
	var out []rune
	switch len(parts) {
	case 0:
	case 1:
		out = []rune(parts[0])
		out = out[:min(2, len(out))]
	default:
		out = []rune{[]rune(parts[0])[0], []rune(parts[1])[0]}
	}
	return strings.ToUpper(string(out))
}

func (u *User) animated() bool { return u.Delay > 0 && u.Previous != u.Balance }

func credits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "−" + b.String()
	}
	return b.String()
}

func signed(n int64) string {
	if n > 0 {
		return "+" + credits(n)
	}
	return credits(n)
}

func percent(f float64) string {
	return strings.Replace(strconv.FormatFloat(f*100, 'f', 1, 64), ".", ",", 1) + " %"
}

func multiplier(perMille int64) string {
	v := float64(perMille) / 1000
	if v == math.Trunc(v) {
		return fmt.Sprintf("×%d", int64(v))
	}
	return "×" + strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

func itoa(n int) string { return strconv.Itoa(n) }

// rank pads a leaderboard position to two digits.
func rank(n int) string { return fmt.Sprintf("%02d", n) }

func kindLabel(kind string) string {
	if kind == "slots" {
		return "Machine à sous"
	}
	return "Table"
}

// gameArt is the artwork of a game, or "" when it has none yet.
func gameArt(key string) string {
	name := "img/jeu-" + key + ".webp"
	if !web.Has(name) {
		return ""
	}
	return web.Asset(name)
}

func i64(n int64) string { return strconv.FormatInt(n, 10) }

func seconds(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) + "s" }

func css(format string, args ...any) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf(format, args...))
}

func netClass(n int64) string {
	switch {
	case n > 0:
		return "up"
	case n < 0:
		return "down"
	}
	return ""
}

func withNext(path, next string) string {
	if next == "" {
		return path
	}
	return path + "?suite=" + url.QueryEscape(next)
}
