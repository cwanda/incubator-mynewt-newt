/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package mfg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/util"
)

const META_MAGIC = 0x3bb2a269
const META_VERSION = 1
const META_TLV_CODE_HASH = 0x01
const META_TLV_CODE_FLASH_AREA = 0x02

const META_HASH_SZ = 32
const META_FOOTER_SZ = 8
const META_TLV_HASH_SZ = META_HASH_SZ
const META_TLV_FLASH_AREA_SZ = 12

type metaHeader struct {
	version uint8
	pad8    uint8
	pad16   uint16
}

type metaFooter struct {
	size  uint16 // Includes header, TLVs, and footer.
	pad16 uint16
	magic uint32
}

type metaTlvHeader struct {
	code uint8
	size uint8
}

type metaTlvFlashArea struct {
	header   metaTlvHeader
	areaId   uint8
	deviceId uint8
	pad16    uint16
	offset   uint32
	size     uint32
}

type metaTlvHash struct {
	header metaTlvHeader
	hash   [META_HASH_SZ]byte
}

func writeElem(elem interface{}, buf *bytes.Buffer) error {
	/* XXX: Assume little endian for now. */
	if err := binary.Write(buf, binary.LittleEndian, elem); err != nil {
		return util.ChildNewtError(err)
	}
	return nil
}

func writeHeader(buf *bytes.Buffer) error {
	hdr := metaHeader{
		version: META_VERSION,
		pad8:    0xff,
		pad16:   0xffff,
	}
	return writeElem(hdr, buf)
}

func writeFooter(buf *bytes.Buffer) error {
	ftr := metaFooter{
		size:  uint16(buf.Len() + META_FOOTER_SZ),
		pad16: 0xffff,
		magic: META_MAGIC,
	}
	return writeElem(ftr, buf)
}

func writeTlvHeader(code uint8, size uint8, buf *bytes.Buffer) error {
	tlvHdr := metaTlvHeader{
		code: code,
		size: size,
	}
	return writeElem(tlvHdr, buf)
}

func writeFlashArea(area flash.FlashArea, buf *bytes.Buffer) error {
	tlv := metaTlvFlashArea{
		header: metaTlvHeader{
			code: META_TLV_CODE_FLASH_AREA,
			size: META_TLV_FLASH_AREA_SZ,
		},
		areaId:   uint8(area.Id),
		deviceId: uint8(area.Device),
		pad16:    0xffff,
		offset:   uint32(area.Offset),
		size:     uint32(area.Size),
	}
	return writeElem(tlv, buf)
}

func writeZeroHash(buf *bytes.Buffer) error {
	tlv := metaTlvHash{
		header: metaTlvHeader{
			code: META_TLV_CODE_HASH,
			size: META_TLV_HASH_SZ,
		},
		hash: [META_HASH_SZ]byte{},
	}
	return writeElem(tlv, buf)
}

// @return						Hash offset, error
func insertMeta(section0Data []byte, flashMap flash.FlashMap) (int, error) {
	buf := &bytes.Buffer{}

	if err := writeHeader(buf); err != nil {
		return 0, err
	}

	for _, area := range flashMap.SortedAreas() {
		if err := writeFlashArea(area, buf); err != nil {
			return 0, err
		}
	}

	if err := writeZeroHash(buf); err != nil {
		return 0, err
	}
	hashSubOff := buf.Len() - META_HASH_SZ

	if err := writeFooter(buf); err != nil {
		return 0, err
	}

	// The meta region gets placed at the very end of the boot loader slot.
	bootArea, ok := flashMap.Areas[flash.FLASH_AREA_NAME_BOOTLOADER]
	if !ok {
		return 0, util.NewNewtError("Required boot loader flash area missing")
	}

	if bootArea.Size < buf.Len() {
		return 0, util.FmtNewtError(
			"Boot loader flash area too small to accommodate meta region; "+
				"boot=%d meta=%d", bootArea.Size, buf.Len())
	}

	metaOff := bootArea.Offset + bootArea.Size - buf.Len()
	for i := metaOff; i < bootArea.Size; i++ {
		if section0Data[i] != 0xff {
			return 0, util.FmtNewtError(
				"Boot loader extends into meta region; "+
					"meta region starts at offset %d", metaOff)
		}
	}

	// Copy the meta region into the manufacturing image.  The meta hash is
	// still zeroed.
	copy(section0Data[metaOff:], buf.Bytes())

	return metaOff + hashSubOff, nil
}

func calcMetaHash(mfgImageBlob []byte, hashOffset int) []byte {
	// Temporarily zero-out old contents for hash calculation.
	oldContents := make([]byte, META_HASH_SZ)
	copy(oldContents, mfgImageBlob[hashOffset:hashOffset+META_HASH_SZ])

	for i := 0; i < META_HASH_SZ; i++ {
		mfgImageBlob[hashOffset+i] = 0
	}

	// Calculate hash.
	hash := sha256.Sum256(mfgImageBlob)

	// Restore old contents.
	copy(mfgImageBlob[hashOffset:hashOffset+META_HASH_SZ], oldContents)

	return hash[:]
}

func fillMetaHash(mfgImageBlob []byte, hashOffset int) {
	hash := calcMetaHash(mfgImageBlob, hashOffset)
	copy(mfgImageBlob[hashOffset:hashOffset+META_HASH_SZ], hash)
}
