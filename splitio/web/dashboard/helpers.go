package dashboard

import "fmt"

// FormatNumber formats int number to string following https://en.wikipedia.org/wiki/Long_and_short_scales
func FormatNumber(n int64) string {

	//Hundred
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	//Thousand
	if n < 1000000 {
		k := float64(n) / float64(1000)
		return fmt.Sprintf("%.2f k", k)
	}

	//Million
	if n < 1000000000 {
		m := float64(n) / float64(1000000)
		return fmt.Sprintf("%.2f M", m)
	}

	//Billion
	if n < 1000000000000 {
		g := float64(n) / float64(1000000000)
		return fmt.Sprintf("%.2f G", g)
	}

	//Trillion
	if n < 1000000000000000 {
		t := float64(n) / float64(1000000000000)
		return fmt.Sprintf("%.2f T", t)
	}

	//Quadrillion
	//if n < 999999999999999999 {
	q := float64(n) / float64(1000000000000000)
	return fmt.Sprintf("%.2f P", q)
	//}

}

// ToRGBAString returns a string representation of rgba colour
func ToRGBAString(r int, g int, b int, a float32) string {
	if a < 1 {
		return fmt.Sprintf("rgba(%d, %d, %d, %.1f)", r, g, b, a)
	}

	return fmt.Sprintf("rgba(%d, %d, %d, %d)", r, g, b, int(a))
}
