package biliutil

var (
	magicStr = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	table    = make(map[byte]int64)
	s        = []int{0, 1, 2, 9, 7, 5, 6, 4, 8, 3, 10, 11}

	BASE = int64(58)
	MAX  = int64(1 << 51)
	LEN  = 12
	XOR  = int64(23442827791579)
	MASK = int64(2251799813685247)
)

func init() {
	for i := range len(magicStr) {
		table[magicStr[i]] = int64(i)
	}
}

func Encode(src int64) string {
	r := []byte("BV1         ")
	it := LEN - 1

	tmp := (src | MAX) ^ XOR

	for tmp != 0 {
		r[s[it]] = magicStr[tmp%BASE]
		tmp /= BASE
		it--
	}

	return string(r)
}

func Decode(src string) int64 {
	r := int64(0)
	bytes := []byte(src)

	for i := 3; i < LEN; i++ {
		r = r*BASE + table[bytes[s[i]]]
	}

	return (r & MASK) ^ XOR
}
