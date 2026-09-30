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
