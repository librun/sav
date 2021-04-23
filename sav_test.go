package sav_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/librun/sav"
)

func TestConvertIntToColumnName(t *testing.T) {
	keys := []int{35, 36, 47, 70, 71, 72, 1259, 1260, 1295, 1296}
	values := []string{"z", "10", "1b", "1y", "1z", "20", "yz", "z0", "zz", "100"}

	for k, key := range keys {
		r := sav.ConvertIntToColumnName(key)
		if strings.ToLower(r) != values[k] {
			t.Error("for " + strconv.Itoa(key) + " get " + r + " wait " + values[k])
		}
	}
}
