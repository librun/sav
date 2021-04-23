package sav

import (
	"fmt"
	"log"
	"os"
)

type (
	Dict struct {
		Name     string
		Type     DictType
		Width    *int
		Decimals *int
		Measure  *string
		Label    string
		Default  *string
		Labels   []Label
	}
	Val struct {
		Name  string
		Value string
	}
	NativeSav struct {
		basename string
		out      *SpssWriter
		lengths  map[string]int
		dict     []Dict
		file     *os.File
	}
)

func GenerateNativeSav(filePath string, dict []Dict, cases [][]Val) error {
	out, err := NewNativeSav(filePath, dict)
	if err != nil {
		return err
	}

	out.findLengths(cases)

	// write dict
	if err := out.WriteDict(); err != nil {
		return err
	}

	for i := range cases {
		if err := out.WriteVal(cases[i]); err != nil {
			return err
		}
	}

	return out.Close()
}

func NewNativeSav(filePath string, dict []Dict) (*NativeSav, error) {
	nv := NativeSav{
		basename: filePath,
		lengths:  make(map[string]int),
		dict:     dict,
	}

	var err error
	nv.file, err = os.Create(filePath + ".sav")
	if err != nil {
		return nil, err
	}

	nv.out = NewSpssWriter(nv.file)
	log.Println("Writing", filePath)

	return &nv, nil
}

func (nv *NativeSav) Close() error {
	if err := nv.out.Finish(); err != nil {
		return err
	}

	return nv.file.Close()
}

func (nv *NativeSav) WriteDict() (err error) {
	for _, d := range nv.dict {
		v := new(Var)
		v.Name = d.Name
		v.Type = d.Type
		v.TypeSize = SPSS_NUMERIC
		v.Label = d.Label
		v.Measure = SPSS_MLVL_NOM

		switch d.Type {
		case DictTypeNumeric:
			v.Print = SPSS_FMT_F
			v.Width = 8
			v.Decimals = 2
			if d.Width != nil {
				v.Width = byte(*d.Width)
			}
			if d.Decimals != nil {
				v.Decimals = byte(*d.Decimals)
			}
		case DictTypeDate:
			v.Print = SPSS_FMT_DATE
			v.Width = 11
			v.Decimals = 0
			v.Measure = SPSS_MLVL_RAT
		case DictTypeDatetime:
			v.Print = SPSS_FMT_DATE_TIME
			v.Width = 20
			v.Decimals = 0
			v.Measure = SPSS_MLVL_RAT
		default: // string
			var width int
			if d.Width != nil {
				width = *d.Width
			} else {
				width, err = nv.getVarLength(v.Name)
				if err != nil {
					return err
				}
			}
			v.TypeSize = int32(width)
			v.Print = SPSS_FMT_A
			v.Width = byte(width)
			if width > 40 {
				v.Width = 40
			}
			v.Decimals = 0
		}

		if d.Default != nil {
			v.HasDefault = true
			v.Default = *d.Default
		}

		if d.Measure != nil {
			switch *d.Measure {
			case "scale":
				v.Measure = SPSS_MLVL_RAT
			case "nominal":
				v.Measure = SPSS_MLVL_NOM
			case "ordinal":
				v.Measure = SPSS_MLVL_ORD
			default:
				return fmt.Errorf("unknown value for measure %s", *d.Measure)
			}
		}
		for _, l := range d.Labels {
			v.Labels = append(v.Labels, Label{Value: l.Value, Desc: l.Desc})
		}
		nv.out.AddVar(v)
	}

	return nv.out.Start(fmt.Sprintf("start write value: %s", nv.basename))
}

func (nv *NativeSav) WriteVal(vals []Val) error {
	nv.out.ClearCase()
	for _, val := range vals {
		nv.out.SetVar(val.Name, val.Value)
	}

	return nv.out.WriteCase()
}

func (nv *NativeSav) findLengths(cases [][]Val) {
	for i := range cases {
		for _, val := range cases[i] {
			if _, ok := nv.lengths[val.Name]; !ok {
				nv.lengths[val.Name] = len(val.Value)

				continue
			}

			if nv.lengths[val.Name] < len(val.Value) {
				nv.lengths[val.Name] = len(val.Value)
			}
		}
	}
}

func (nv *NativeSav) getVarLength(name string) (int, error) {
	le, found := nv.lengths[name]
	if !found {
		return 0, fmt.Errorf("can not find variable %s", name)
	}

	return le, nil
}
