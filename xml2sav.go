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
	"bytes"
	"io"
	"log"
)

const defaultStringLength = 2048

func GenerateSav(data []byte, filePath string) error {
	in := bytes.NewReader(data)
	var lengths VarLengths
	var err error

	if lengths, err = findVarLengths(in); err != nil {
		log.Println(err)
	}
	if _, errSeek := in.Seek(0, io.SeekStart); errSeek != nil {
		return errSeek
	}

	return parseXSav(in, filePath, lengths)
}
