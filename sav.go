/*
xml2sav - converts a custom xml document to a SPSS binary file.
Copyright (C) 2016-2017 A.J. Jessurun

This file is part of xml2sav.

Xml2sav is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

Xml2sav is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with xml2sav.  If not, see <http://www.gnu.org/licenses/>.
*/
package sav

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxStringLength     = 1024 * 50
	maxPrintStringWidth = 40
	TimeOffset          = 12219379200
	SPSS_NUMERIC        = 0

	SPSS_FMT_A         = 1
	SPSS_FMT_F         = 5
	SPSS_FMT_DATE      = 20
	SPSS_FMT_DATE_TIME = 22

	SPSS_MLVL_NOM = 1
	SPSS_MLVL_ORD = 2
	SPSS_MLVL_RAT = 3
)

type Label struct {
	Value string
	Desc  string
}

type DictType int

const (
	DictTypeNumeric DictType = iota
	DictTypeDate
	DictTypeDatetime
	DictTypeString
)

type Var struct {
	ColumnIndex int32
	Index       int32
	Name        string
	ShortName   string
	Type        DictType
	TypeSize    int32
	Print       byte
	Width       byte
	Decimals    byte
	Measure     int32
	Label       string
	Default     string
	HasDefault  bool
	Labels      []Label
	Value       string
	HasValue    bool
	Segments    int // how many segments
}

// SegmentWidth returns the width of the given segment
func (v *Var) SegmentWidth(index int) int32 {
	if v.TypeSize <= 255 {
		return v.TypeSize
	}
	if index < v.Segments-1 {
		return 255
	}
	return v.TypeSize - int32(v.Segments-1)*252
}

var endian = binary.LittleEndian

type SpssWriter struct {
	*bufio.Writer                    // Buffered writer
	seeker           io.WriteSeeker  // Original writer
	bytecode         *BytecodeWriter // Special writer for compressed cases
	Dict             []*Var          // Variables
	DictMap          map[string]*Var // Long variable names index
	ShortMap         map[string]*Var // Short variable names index
	Count            int32           // Number of cases
	Index            int32
	ColumnIndex      int32
	IgnoreMissingVar bool
}

func NewSpssWriter(w io.WriteSeeker) *SpssWriter {
	out := &SpssWriter{
		seeker:   w,
		Writer:   bufio.NewWriter(w),
		DictMap:  make(map[string]*Var),
		ShortMap: make(map[string]*Var),
		Index:    1,
	}
	out.bytecode = NewBytecodeWriter(out.Writer, 100.0)
	return out
}

func stob(s string, l int) []byte {
	if len(s) > l {
		s = s[:l]
	} else if len(s) < l {
		s += strings.Repeat(" ", l-len(s))
	}
	return []byte(s)
}

func stobp(s string, l int, pad byte) []byte {
	if len(s) > l {
		s = s[:l]
	} else if len(s) < l {
		s += strings.Repeat(string([]byte{pad}), l-len(s))
	}
	return []byte(s)
}

func trim(s string, l int) string {
	if len(s) > l {
		return s[:l]
	}
	return s
}

func ConvertIntToColumnName(s int) (result string) {
	for {
		denom := s % 36
		result = getAlpabetByInt(denom) + result

		if s/36 >= 1 {
			s = (s - denom) / 36
		} else {
			break
		}
	}

	return result
}

func getAlpabetByInt(s int) string {
	switch s {
	case 10:
		return "a"
	case 11:
		return "b"
	case 12:
		return "c"
	case 13:
		return "d"
	case 14:
		return "e"
	case 15:
		return "f"
	case 16:
		return "g"
	case 17:
		return "h"
	case 18:
		return "i"
	case 19:
		return "j"
	case 20:
		return "k"
	case 21:
		return "l"
	case 22:
		return "m"
	case 23:
		return "n"
	case 24:
		return "o"
	case 25:
		return "p"
	case 26:
		return "q"
	case 27:
		return "r"
	case 28:
		return "s"
	case 29:
		return "t"
	case 30:
		return "u"
	case 31:
		return "v"
	case 32:
		return "w"
	case 33:
		return "x"
	case 34:
		return "y"
	case 35:
		return "z"
	default:
		return strconv.Itoa(s)
	}
}

func atof(s string) float64 {
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		log.Fatalln(err)
	}
	return v
}

func elementCount(width int32) int32 {
	return ((width - 1) / 8) + 1
}

var cleanVarNameRegExp = regexp.MustCompile(`[^A-Za-z0-9#\$_\.]`)

func cleanVarName(n string) string {
	n = cleanVarNameRegExp.ReplaceAllLiteralString(n, "")
	if len(n) == 0 {
		n = "illegal"
	}
	if (n[0] < 'a' || n[0] > 'z') && (n[0] < 'A' || n[0] > 'Z') {
		n = "@" + n
	}
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

func (out *SpssWriter) caseSize() int32 {
	size := int32(0)
	for _, v := range out.Dict {
		for s := 0; s < v.Segments; s++ {
			size += elementCount(v.SegmentWidth(s))
		}
	}
	return size
}

func (out *SpssWriter) Seek(offset int64, whence int) (int64, error) {
	return out.seeker.Seek(offset, whence)
}

func (out *SpssWriter) VarCount() int32 {
	var count int32
	for _, v := range out.Dict {
		count += int32(v.Segments)
	}
	return count
}

// writeString writes a string value for a case
func (out *SpssWriter) writeString(v *Var, val string) error {
	for s := 0; s < v.Segments; s++ {
		var p string
		if len(val) > 255 {
			p = val[:255]
			val = val[255:]
		} else {
			p = val
			val = ""
		}

		if err := out.bytecode.WriteString(p, int(elementCount(v.SegmentWidth(s)))); err != nil {
			return err
		}
	}

	return nil
}

func (out *SpssWriter) headerRecord(fileLabel string) error {
	c := time.Now()

	if _, err := out.Write(stob("$FL2", 4)); err != nil { // rec_tyoe
		return err
	}

	if _, err := out.Write(stob("@(#) SPSS DATA FILE - xml2sav 2.0", 60)); err != nil { // prod_name
		return err
	}

	if err := binary.Write(out, endian, int32(2)); err != nil { // layout_code
		return err
	}

	if err := binary.Write(out, endian, out.caseSize()); err != nil { // nominal_case_size
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // compression
		return err
	}

	if err := binary.Write(out, endian, int32(0)); err != nil { // weight_index
		return err
	}

	if err := binary.Write(out, endian, int32(-1)); err != nil { // ncases
		return err
	}

	if err := binary.Write(out, endian, float64(100)); err != nil { // bias
		return err
	}

	if _, err := out.Write(stob(c.Format("02 Jan 06"), 9)); err != nil { // creation_date
		return err
	}

	if _, err := out.Write(stob(c.Format("15:04:05"), 8)); err != nil { // creation_time
		return err
	}

	if _, err := out.Write(stob(fileLabel, 64)); err != nil { // file_label
		return err
	}

	if _, err := out.Write(stob("\x00\x00\x00", 3)); err != nil { // padding
		return err
	}

	return nil
}

// If you use a buffer, supply it as the flusher argument
// After this close the file
func (out *SpssWriter) updateHeaderNCases() error {
	out.bytecode.Flush()
	out.Flush()
	if _, err := out.Seek(80, 0); err != nil {
		return err
	}

	return binary.Write(out.seeker, endian, out.Count) // ncases in headerRecord
}

func (out *SpssWriter) variableRecords() error {
	for _, v := range out.Dict {
		for segment := 0; segment < v.Segments; segment++ {
			width := v.SegmentWidth(segment)
			if err := binary.Write(out, endian, int32(2)); err != nil { // rec_type
				return err
			}

			if err := binary.Write(out, endian, width); err != nil { // type (0 or strlen)
				return err
			}
			if segment == 0 && len(v.Label) > 0 {
				if err := binary.Write(out, endian, int32(1)); err != nil { // has_var_label
					return err
				}
			} else {
				if err := binary.Write(out, endian, int32(0)); err != nil { // has_var_label
					return err
				}
			}
			if err := binary.Write(out, endian, int32(0)); err != nil { // n_missing_values
				return err
			}

			var format int32
			if v.TypeSize > 0 { // string
				format = int32(v.Print)<<16 | int32(width)<<8
			} else { // number
				format = int32(v.Print)<<16 | int32(v.Width)<<8 | int32(v.Decimals)
			}
			if err := binary.Write(out, endian, format); err != nil { // print
				return err
			}

			if err := binary.Write(out, endian, format); err != nil { // write
				return err
			}

			if segment == 0 { // first var
				v.ShortName = out.makeShortName(v, segment)
				if _, err := out.Write(stob(v.ShortName, 8)); err != nil { // name
					return err
				}

				if len(v.Label) > 0 {
					if err := binary.Write(out, endian, int32(len(v.Label))); err != nil { // label_len
						return err
					}

					if _, err := out.Write([]byte(v.Label)); err != nil { // label
						return err
					}

					pad := (4 - len(v.Label)) % 4
					if pad < 0 {
						pad += 4
					}
					for i := 0; i < pad; i++ {
						if _, err := out.Write([]byte{0}); err != nil { // pad out until multiple of 32 bit
							return err
						}
					}
				}
			} else { // segment > 0
				if _, err := out.Write(stob(out.makeShortName(v, segment), 8)); err != nil { // name (a fresh new one)
					return err
				}
			}

			if width > 8 { // handle long string
				count := int(elementCount(width) - 1) // number of extra vars to store string
				for i := 0; i < count; i++ {
					if err := binary.Write(out, endian, int32(2)); err != nil { // rec_type
						return err
					}

					if err := binary.Write(out, endian, int32(-1)); err != nil { // extended string part
						return err
					}

					if err := binary.Write(out, endian, int32(0)); err != nil { // has_var_label
						return err
					}

					if err := binary.Write(out, endian, int32(0)); err != nil { // n_missing_valuess
						return err
					}

					if err := binary.Write(out, endian, int32(0)); err != nil { // print
						return err
					}

					if err := binary.Write(out, endian, int32(0)); err != nil { // write
						return err
					}

					if _, err := out.Write(stob("        ", 8)); err != nil { // name
						return err
					}
				}
			}
		}
	}

	return nil
}

func (out *SpssWriter) valueLabelRecords() error {
	for _, v := range out.Dict {
		if len(v.Labels) > 0 && v.TypeSize <= 8 {
			if err := binary.Write(out, endian, int32(3)); err != nil { // rec_type
				return err
			}

			if err := binary.Write(out, endian, int32(len(v.Labels))); err != nil { // label_count
				return err
			}

			for _, label := range v.Labels {
				if v.TypeSize == 0 {
					if err := binary.Write(out, endian, atof(label.Value)); err != nil { // value
						return err
					}
				} else {
					if err := binary.Write(out, endian, stob(label.Value, 8)); err != nil { // value
						return err
					}
				}
				l := len(label.Desc)
				if l > 120 {
					l = 120
				}
				if err := binary.Write(out, endian, byte(l)); err != nil { // label_len
					return err
				}

				if _, err := out.Write(stob(label.Desc, l)); err != nil { // label
					return err
				}

				pad := (8 - l - 1) % 8
				if pad < 0 {
					pad += 8
				}
				for i := 0; i < pad; i++ {
					if _, err := out.Write([]byte{32}); err != nil {
						return err
					}
				}
			}

			if err := binary.Write(out, endian, int32(4)); err != nil { // rec_type
				return err
			}

			if err := binary.Write(out, endian, int32(1)); err != nil { // var_count
				return err
			}

			if err := binary.Write(out, endian, int32(v.Index)); err != nil { // vars
				return err
			}
		}
	}

	return nil
}

func (out *SpssWriter) machineIntegerInfoRecord() error {
	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(3)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(4)); err != nil { // size
		return err
	}

	if err := binary.Write(out, endian, int32(8)); err != nil { // count
		return err
	}

	if err := binary.Write(out, endian, int32(0)); err != nil { // version_major
		return err
	}

	if err := binary.Write(out, endian, int32(10)); err != nil { // version_minor
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // version_revision
		return err
	}

	if err := binary.Write(out, endian, int32(-1)); err != nil { // machine_code
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // floating_point_rep
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // compression_code
		return err
	}

	if err := binary.Write(out, endian, int32(2)); err != nil { // endianness
		return err
	}

	if err := binary.Write(out, endian, int32(65001)); err != nil { // character_code
		return err
	}

	return nil
}

func (out *SpssWriter) machineFloatingPointInfoRecord() error {
	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(4)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(8)); err != nil { // size
		return err
	}

	if err := binary.Write(out, endian, int32(3)); err != nil { // count
		return err
	}

	if err := binary.Write(out, endian, float64(-math.MaxFloat64)); err != nil { // sysmis
		return err
	}

	if err := binary.Write(out, endian, float64(math.MaxFloat64)); err != nil { // highest
		return err
	}

	if err := binary.Write(out, endian, float64(-math.MaxFloat64)); err != nil { // lowest
		return err
	}

	return nil
}

func (out *SpssWriter) variableDisplayParameterRecord() error {
	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(11)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(4)); err != nil { // size
		return err
	}

	if err := binary.Write(out, endian, out.VarCount()*3); err != nil { // count
		return err
	}

	for _, v := range out.Dict {
		for s := 0; s < v.Segments; s++ {
			if err := binary.Write(out, endian, v.Measure); err != nil { // measure
				return err
			}

			if v.TypeSize > 0 {
				if s != 0 {
					if err := binary.Write(out, endian, int32(8)); err != nil { // width
						return err
					}
				} else if v.TypeSize <= int32(maxPrintStringWidth) {
					if err := binary.Write(out, endian, v.TypeSize); err != nil { // width
						return err
					}
				} else {
					if err := binary.Write(out, endian, int32(maxPrintStringWidth)); err != nil { // width
						return err
					}
				}
				if err := binary.Write(out, endian, int32(0)); err != nil { // alignment (left)
					return err
				}
			} else {
				if err := binary.Write(out, endian, int32(8)); err != nil { // width
					return err
				}

				if err := binary.Write(out, endian, int32(1)); err != nil { // alignment (right)
					return err
				}
			}
		}
	}

	return nil
}

func (out *SpssWriter) longVarNameRecords() error {
	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(13)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // size
		return err
	}

	buf := bytes.Buffer{}
	for i, v := range out.Dict {
		if _, err := buf.Write([]byte(v.ShortName)); err != nil {

			return err
		}

		if _, err := buf.Write([]byte("=")); err != nil {
			return err
		}

		if _, err := buf.Write([]byte(v.Name)); err != nil {
			return err
		}

		if i < len(out.Dict)-1 {
			if _, err := buf.Write([]byte{9}); err != nil {
				return err
			}
		}
	}
	if err := binary.Write(out, endian, int32(buf.Len())); err != nil {
		return err
	}
	if _, err := out.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func (out *SpssWriter) veryLongStringRecord() error {
	b := false
	for _, v := range out.Dict {
		if v.Segments > 1 {
			b = true
			break
		}
	}

	if !b {
		// There are no very long strings so don't write the record
		return nil
	}

	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(14)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // size
		return err
	}

	buf := bytes.Buffer{}
	for _, v := range out.Dict {
		if v.Segments > 1 {
			if _, err := buf.Write([]byte(v.ShortName)); err != nil {
				return err
			}

			if _, err := buf.Write([]byte("=")); err != nil {
				return err
			}

			if _, err := buf.Write(stobp(strconv.Itoa(int(v.TypeSize)), 5, 0)); err != nil {
				return err
			}

			if _, err := buf.Write([]byte{0, 9}); err != nil {
				return err
			}
		}
	}

	if err := binary.Write(out, endian, int32(buf.Len())); err != nil { // count
		return err
	}

	if _, err := out.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func (out *SpssWriter) encodingRecord() error {
	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(20)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // size
		return err
	}

	if err := binary.Write(out, endian, int32(5)); err != nil { // filler
		return err
	}

	if _, err := out.Write(stob("UTF-8", 5)); err != nil { // encoding
		return err
	}

	return nil
}

func (out *SpssWriter) longStringValueLabelsRecord() error {
	// Check if we have any
	any := false
	for _, v := range out.Dict {
		if len(v.Labels) > 0 && v.TypeSize > 8 {
			any = true
			break
		}
	}
	if !any {
		return nil
	}

	// Create record
	buf := new(bytes.Buffer)
	for _, v := range out.Dict {
		if len(v.Labels) > 0 && v.TypeSize > 8 {
			if err := binary.Write(buf, endian, int32(len(v.ShortName))); err != nil { // var_name_len
				return err
			}

			if _, err := buf.Write([]byte(v.ShortName)); err != nil { // var_name
				return err
			}

			if err := binary.Write(buf, endian, v.TypeSize); err != nil { // var_width
				return err
			}

			if err := binary.Write(buf, endian, int32(len(v.Labels))); err != nil { // n_labels
				return err
			}

			for _, l := range v.Labels {
				if err := binary.Write(buf, endian, int32(len(l.Value))); err != nil { // value_len
					return err
				}

				if _, err := buf.Write([]byte(l.Value)); err != nil { // value
					return err
				}

				if err := binary.Write(buf, endian, int32(len(l.Desc))); err != nil { // label_len
					return err
				}

				if _, err := buf.Write([]byte(l.Desc)); err != nil { //label
					return err
				}
			}
		}
	}

	if err := binary.Write(out, endian, int32(7)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(21)); err != nil { // subtype
		return err
	}

	if err := binary.Write(out, endian, int32(1)); err != nil { // size
		return err
	}

	if err := binary.Write(out, endian, int32(buf.Len())); err != nil { // count
		return err
	}

	if _, err := out.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func (out *SpssWriter) terminationRecord() error {
	if err := binary.Write(out, endian, int32(999)); err != nil { // rec_type
		return err
	}

	if err := binary.Write(out, endian, int32(0)); err != nil { // filler
		return err
	}

	return nil
}

func (out *SpssWriter) makeShortName(v *Var, segment int) string {
	baseName := strings.ToUpper(trim(v.Name, 5))
	short := baseName + "0"

	for {
		_, found := out.ShortMap[short]
		if !found {
			break
		}

		appendText := strings.ToUpper(ConvertIntToColumnName(segment))
		segment++

		if len(appendText) > 3 { // Come up with random name
			short = "@" + strconv.Itoa(rand.Int()%10000000)
		} else {
			short = baseName + appendText
		}
	}
	out.ShortMap[short] = v

	return short
}

func (out *SpssWriter) AddVar(v *Var) {
	if v.TypeSize > int32(maxStringLength) {
		log.Fatalf("maximum length for a variable is %d, %s is %d", maxStringLength, v.Name, v.TypeSize)
	}

	// Clean variable name
	origName := v.Name
	name := cleanVarName(v.Name)
	if name != v.Name {
		log.Printf("Change variable name '%s' to '%s'\n", v.Name, name)
		v.Name = name
	}

	if _, found := out.DictMap[origName]; found {
		log.Fatalln("Adding duplicate variable named", origName)
	}

	v.Segments = 1
	if v.TypeSize > 255 {
		v.Segments = (int(v.TypeSize) + 251) / 252
	}

	v.Index = out.Index
	for i := 0; i < v.Segments; i++ {
		out.Index += elementCount(v.SegmentWidth(i))
	}
	out.ColumnIndex++
	v.ColumnIndex = out.ColumnIndex

	out.Dict = append(out.Dict, v)
	out.DictMap[origName] = v
}

func (out *SpssWriter) ClearCase() {
	for _, v := range out.Dict {
		v.Value = ""
		v.HasValue = false
	}
}

func (out *SpssWriter) SetVar(name, value string) {
	v, found := out.DictMap[name]
	if !found {
		if out.IgnoreMissingVar {
			return
		}
		log.Fatalln("Can not find the variable named in dictionary", name)
	}
	v.Value = value
	v.HasValue = true
}

func (out *SpssWriter) WriteCase() error {
	for _, v := range out.Dict {
		if v.HasValue || v.HasDefault {
			var val string
			if v.HasValue {
				val = v.Value
			} else {
				val = v.Default
			}

			if v.TypeSize > 0 { // string
				if len(val) > int(v.TypeSize) {
					val = val[:v.TypeSize]
					log.Printf("Truncated string for %s: %s\n", v.Name, val)
				}
				if err := out.writeString(v, val); err != nil {
					return err
				}
			} else if v.Print == SPSS_FMT_DATE {
				if val == "" {
					if err := binary.Write(out, endian, -math.MaxFloat64); err != nil { // Write missing
						return err
					}
				} else {
					t, err := time.Parse("2-Jan-2006", v.Value)
					if err != nil {
						log.Printf("Problem pasing value for %s: %s - set as missing\n", v.Name, err)
						if err := out.bytecode.WriteMissing(); err != nil {
							return err
						}
					} else {
						if err := out.bytecode.WriteNumber(float64(t.Unix() + TimeOffset)); err != nil {
							return err
						}
					}
				}
			} else if v.Print == SPSS_FMT_DATE_TIME {
				if val == "" {
					if err := out.bytecode.WriteMissing(); err != nil {
						return err
					}
				} else {
					t, err := time.Parse("2-Jan-2006 15:04:05", v.Value)
					if err != nil {
						log.Printf("Problem pasing value for %s: %s - set as missing\n", v.Name, err)
						if err := out.bytecode.WriteMissing(); err != nil {
							return err
						}
					} else {
						if err := out.bytecode.WriteNumber(float64(t.Unix() + TimeOffset)); err != nil {
							return err
						}
					}
				}
			} else { // number
				if val == "" {
					if err := out.bytecode.WriteMissing(); err != nil {
						return err
					}
				} else {
					f, err := strconv.ParseFloat(val, 64)
					if err != nil {
						log.Printf("Problem pasing value for %s: %s - set as missing\n", v.Name, err)
						if err := out.bytecode.WriteMissing(); err != nil {
							return err
						}
					} else {
						if err := out.bytecode.WriteNumber(f); err != nil {
							return err
						}
					}
				}
			}
		} else { // Write missing value
			if v.TypeSize > 0 {
				if err := out.writeString(v, ""); err != nil {
					return err
				}
			} else {
				if err := out.bytecode.WriteMissing(); err != nil {
					return err
				}
			}
		}
	}
	out.Count++

	return nil
}

func (out *SpssWriter) Start(fileLabel string) error {
	if err := out.headerRecord(fileLabel); err != nil {
		return err
	}

	if err := out.variableRecords(); err != nil {
		return err
	}

	if err := out.valueLabelRecords(); err != nil {
		return err
	}

	if err := out.machineIntegerInfoRecord(); err != nil {
		return err
	}

	if err := out.machineFloatingPointInfoRecord(); err != nil {
		return err
	}

	if err := out.variableDisplayParameterRecord(); err != nil {
		return err
	}

	if err := out.longVarNameRecords(); err != nil {
		return err
	}

	if err := out.veryLongStringRecord(); err != nil {
		return err
	}

	if err := out.encodingRecord(); err != nil {
		return err
	}

	if err := out.longStringValueLabelsRecord(); err != nil {
		return err
	}

	if err := out.terminationRecord(); err != nil {
		return err
	}

	return nil
}

func (out *SpssWriter) Finish() error {
	return out.updateHeaderNCases()
}
