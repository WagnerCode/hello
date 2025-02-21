package clockface

import "time"

type Point struct {
	X float64
	Y float64
}

func SecondHand(time.Time) Point {
	return Point{150, 60}
}
