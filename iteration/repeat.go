package iteration

import (
	"strings"
)

func Repeat(character string, number int) string {
	var repeated string
	for i := 0; i < number; i++ {
		repeated += character
	}
	return repeated
}

func ReplaceCharacter(source string, character string, replacement string, times int) string {
	return strings.Replace(source, character, replacement, times)
}
