package routing

import (
	"math/big"
	"strings"
	"time"
)

// The 0.437.14 compiled Mojo policy parses unsigned whole/fraction digits
// with a UInt128 limit *before* capping retry duration at five minutes.
// Floating-point parsing cannot reproduce the overflow and 1 ms rounding
// decisions for extreme values and small fractions.
var maxRetryNumber043714 = func() *big.Int {
	n := new(big.Int).Lsh(big.NewInt(1), 128)
	return n.Sub(n, big.NewInt(1))
}()

func retryAfterTaggedDecimal043714(number string, unit time.Duration) (time.Duration, bool) {
	wholeText, fraction, hasDot := strings.Cut(number, ".")
	if wholeText == "" || hasDot && strings.Contains(fraction, ".") {
		return 0, false
	}
	for _, raw := range []string{wholeText, fraction} {
		for index := 0; index < len(raw); index++ {
			if raw[index] < '0' || raw[index] > '9' {
				return 0, false
			}
		}
	}
	whole, ok := new(big.Int).SetString(wholeText, 10)
	if !ok || whole.Cmp(maxRetryNumber043714) > 0 {
		return 0, false
	}

	total := new(big.Int).Set(whole)
	anyFraction := false
	firstMillis := int64(0)
	digits := 0
	roundUp := false
	for index := 0; index < len(fraction); index++ {
		digit := int64(fraction[index] - '0')
		if digit != 0 {
			anyFraction = true
		}
		if digits < 3 {
			firstMillis = firstMillis*10 + digit
		} else if digit != 0 {
			roundUp = true
		}
		digits++
	}
	for digits < 3 {
		firstMillis *= 10
		digits++
	}
	if roundUp {
		firstMillis++
	}
	if unit == time.Second {
		total.Mul(total, big.NewInt(1000))
		total.Add(total, big.NewInt(firstMillis))
	} else if unit == time.Millisecond {
		if anyFraction {
			total.Add(total, big.NewInt(1))
		}
	} else {
		return 0, false
	}
	if total.Sign() == 0 || total.Cmp(maxRetryNumber043714) > 0 {
		return 0, false
	}
	capMillis := int64(maxRetryAfter / time.Millisecond)
	if total.Cmp(big.NewInt(capMillis)) > 0 {
		return maxRetryAfter, true
	}
	return time.Duration(total.Int64()) * time.Millisecond, true
}

func retryAfterASCIIFind043714(text, marker string) int {
	if len(text) < len(marker) {
		return -1
	}
	for start := 0; start+len(marker) <= len(text); start++ {
		ok := true
		for i := 0; i < len(marker); i++ {
			b := text[start+i]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if b != marker[i] {
				ok = false
				break
			}
		}
		if ok {
			return start
		}
	}
	return -1
}
