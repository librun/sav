package sav_test

import (
	"fmt"
	"log"
	"strconv"
	"testing"

	"github.com/librun/sav"
)

func TestCreateSavFileLongText(t *testing.T) {
	stringWidth := 12000

	savDict := []sav.Dict{}
	vals := []sav.Val{}

	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			dictName := fmt.Sprintf("q%04d", i+1) + "_other_" + strconv.Itoa(j+1)
			savDict = append(savDict, sav.Dict{
				Type:  sav.DictTypeString,
				Name:  dictName,
				Width: &stringWidth,
			})

			vals = append(vals, sav.Val{
				Name:  dictName,
				Value: "longtext",
			})
		}
	}

	docName := fmt.Sprintf("%s/%s", "./", "test")
	savFile, err := sav.NewNativeSav(docName, savDict)
	if err != nil {
		log.Fatal(err)
	}

	if errWriteDir := savFile.WriteDict(); errWriteDir != nil {
		log.Fatal(err)
	}

	if err := savFile.WriteVal(vals); err != nil {
		log.Fatal(err)
	}

	if err := savFile.Close(); err != nil {
		log.Fatal(err)
	}
}
