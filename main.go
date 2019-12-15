package main

import (
	"encoding/binary"
	"fmt"
	"github.com/shumon84/binutil"
	"io"
	"log"
	"os"
)

func main() {
	file, err := os.Open("testdata/sample.blend")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	blend, err := ReadBlend(file)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(blend.Header)
	dna := blend.FileBlocks[len(blend.FileBlocks)-2].Data.(DNA1)
	fmt.Println(dna)

	for i, block := range blend.FileBlocks {
		structure := dna.Structures[block.Header.SDNAIndex]
		fmt.Printf("File Block %d: %s %dbyte x %d (%dbyte)\n", i, dna.Types[structure.Type], dna.TLens[structure.Type], block.Header.Count, block.Header.Size)
	}
}

type Blend struct {
	Header     Header
	FileBlocks []FileBlock
}

func ReadBlend(rs io.ReadSeeker) (*Blend, error) {
	r := binutil.NewReaderWithEndian(rs, binary.LittleEndian)
	blend := &Blend{}
	header := Header{}
	if err := r.Read(&header); err != nil {
		return nil, err
	}
	blend.Header = header

	isNotENDB := true
	for isNotENDB {
		header := FileBlockHeader{}
		if err := r.Read(&header); err != nil {
			return nil, err
		}
		isNotENDB = string(header.Code[:]) != "ENDB"

		var data FileBlockData
		if string(header.Code[:]) != "DNA1" {
			// DNA1じゃない場合は読み込みをスキップ
			if _, err := r.Seek(int64(header.Size), io.SeekCurrent); err != nil {
				return nil, err
			}
			data = DummyData{}
		} else {
			// DNA1を読み込む
			identifier := [4]byte{}
			if err := r.Read(&identifier); err != nil {
				return nil, err
			}
			nameIdentifier := [4]byte{}
			if err := r.Read(&nameIdentifier); err != nil {
				return nil, err
			}
			numOfNames, err := r.UInt32()
			if err != nil {
				return nil, err
			}
			names, err := r.Strings(int(numOfNames))
			if err != nil {
				return nil, err
			}
			if err := Align(r, 4); err != nil {
				return nil, err
			}
			typeIdentifier := [4]byte{}
			if err := r.Read(&typeIdentifier); err != nil {
				return nil, err
			}
			numOfTypes, err := r.UInt32()
			if err != nil {
				return nil, err
			}
			types, err := r.Strings(int(numOfTypes))
			if err != nil {
				return nil, err
			}
			if err := Align(r, 4); err != nil {
				return nil, err
			}
			typeLengthIdentifier := [4]byte{}
			if err := r.Read(&typeLengthIdentifier); err != nil {
				return nil, err
			}
			typeLengths, err := r.UInt16s(int(numOfTypes))
			if err != nil {
				return nil, err
			}
			if err := Align(r, 4); err != nil {
				return nil, err
			}
			structureIdentifier := [4]byte{}
			if err := r.Read(&structureIdentifier); err != nil {
				return nil, err
			}
			numOfStructures, err := r.UInt32()
			if err != nil {
				return nil, err
			}
			structures := make([]DNAStructure, numOfStructures)
			for i := 0; i < int(numOfStructures); i++ {
				structureType, err := r.UInt16()
				if err != nil {
					return nil, err
				}
				numOfFields, err := r.UInt16()
				if err != nil {
					return nil, err
				}
				fields := make([]DNAStructureField, numOfFields)
				for i := 0; i < int(numOfFields); i++ {
					fieldType, err := r.UInt16()
					if err != nil {
						return nil, err
					}
					fieldName, err := r.UInt16()
					if err != nil {
						return nil, err
					}
					fields[i] = DNAStructureField{
						Type: fieldType,
						Name: fieldName,
					}
				}
				structures[i] = DNAStructure{
					Type:        structureType,
					NumOfFields: numOfFields,
					Fields:      fields,
				}
			}
			data = DNA1{
				Identifier:          identifier,
				NameIdentifier:      nameIdentifier,
				NumOfNames:          numOfNames,
				Names:               names,
				TypeIdentifier:      typeIdentifier,
				NumOfTypes:          numOfTypes,
				Types:               types,
				TLenIdentifier:      typeLengthIdentifier,
				TLens:               typeLengths,
				StructureIdentifier: structureIdentifier,
				NumOfStructures:     numOfStructures,
				Structures:          structures,
			}
		}

		blend.FileBlocks = append(blend.FileBlocks, FileBlock{
			Header: header,
			Data:   data,
		})
	}
	return blend, nil
}

type Header struct {
	Identifier    [7]byte // ファイルの識別子。必ず[7]byte("BLENDER")になる。
	PointerSize   byte    // ポインタのサイズ。'_'とき32bit、'-'とき64bit。
	Endianness    byte    // エンディアン。'v'のときリトルエンディアン。'V'のときビッグエンディアン。
	VersionNumber [3]byte // Blenderのバージョン番号からドットを抜いたもの。例えば"2.81"の場合は"281"。
}

func (h Header) String() string {
	str := ""
	str += fmt.Sprintln("Header:")
	str += fmt.Sprintln("    Identifier:", string(h.Identifier[:]))
	if h.PointerSize == '_' {
		str += fmt.Sprintln("    PointerSize: 32bit")
	} else if h.PointerSize == '-' {
		str += fmt.Sprintln("    PointerSize: 64bit")
	} else {
		str += fmt.Sprintln("    PointerSize: Undefined")
	}
	if h.Endianness == 'v' {
		str += fmt.Sprintln("    Endianness: Little endian")
	} else if h.Endianness == 'V' {
		str += fmt.Sprintln("    Endianness: Big endian")
	} else {
		str += fmt.Sprintln("    Endianness: Undefined")
	}
	str += fmt.Sprintf("    VersionNumber: %s.%s %v\n", string(h.VersionNumber[:1]), string(h.VersionNumber[1:]), h.VersionNumber)
	return str
}

type FileBlock struct {
	Header FileBlockHeader
	Data   FileBlockData
}
type FileBlockHeader struct {
	Code             [4]byte // ファイルブロックの識別子。
	Size             uint32  // あとに続くファイルブロックデータの長さ
	OldMemoryAddress uint64  // このファイルが保存されたときにこのファイルブロックが配置されていたメモリアドレス
	SDNAIndex        uint32  // SDNAのインデックス
	Count            uint32  // このファイルブロックに存在する構造体の数
}

func (fbh FileBlockHeader) String() string {
	str := ""
	str += fmt.Sprintln("File Block Header:")
	str += fmt.Sprintln("    Code:", string(fbh.Code[:]), fbh.Code)
	str += fmt.Sprintln("    Size:", fbh.Size)
	str += fmt.Sprintf("    OldMemoryAddress: 0x%X\n", fbh.OldMemoryAddress)
	str += fmt.Sprintln("    SDNAIndex:", fbh.SDNAIndex)
	str += fmt.Sprintln("    Count:", fbh.Count)
	return str
}

type FileBlockData interface {
	fmt.Stringer
}
type DummyData struct{}

func (dd DummyData) String() string {
	return "FileBlockData: (dummy)\n"
}

type DNA1 struct {
	Identifier          [4]byte // 必ず"SDNA"
	NameIdentifier      [4]byte // 必ず"NAME"
	NumOfNames          uint32
	Names               []string
	TypeIdentifier      [4]byte // 必ず"TYPE"
	NumOfTypes          uint32
	Types               []string
	TLenIdentifier      [4]byte  // 必ず"TLEN"
	TLens               []uint16 // len(TLens) == NumOfTypes
	StructureIdentifier [4]byte  // 必ず"STRC"
	NumOfStructures     uint32
	Structures          []DNAStructure
}

func (d DNA1) String() string {
	str := ""
	str += fmt.Sprintln("Structures:")
	for i, structure := range d.Structures {
		str += fmt.Sprintln("    Type", i, ":", d.Types[structure.Type], d.TLens[structure.Type], "byte")
		for i, field := range structure.Fields {
			str += fmt.Sprintln("        Field", i, ":", d.Types[field.Type], d.Names[field.Name])
		}
	}
	return str
}

type DNAStructure struct {
	Type        uint16
	NumOfFields uint16
	Fields      []DNAStructureField
}

type DNAStructureField struct {
	Type uint16
	Name uint16
}

func Align(r io.Seeker, alignment int64) error {
	offset, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil
	}
	trim := offset % alignment
	if trim == 0 {
		return nil
	}
	if _, err := r.Seek(4-trim, io.SeekCurrent); err != nil {
		return err
	}
	return nil
}
